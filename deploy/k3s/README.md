# NAV в k3s

В каталоге находится одновузловой deployment-профиль для локального k3s:

- `nav-api` — UI и основной API;
- `nav-auth` — auth/account API;
- `nav-calc-worker` — обработчик расчётов;
- PostgreSQL 17 — постоянное состояние приложения и GSN;
- RabbitMQ 4.1 — очередь расчётов;
- Redis 8 — кэш GSN;
- Prometheus, Grafana, Loki, Tempo, OpenTelemetry Collector и Grafana Alloy —
  метрики, дашборды, логи и распределённые трейсы;
- Traefik Ingress — единая точка входа `http://nav.local`.

PostgreSQL, RabbitMQ и Redis используют PVC стандартного storage class k3s
(`local-path`, если конфигурация k3s не менялась). Это профиль для одного узла,
а не HA-конфигурация.

## Требования

- k3s и встроенный Traefik;
- чтение kubeconfig k3s текущим пользователем;
- Docker или Podman внутри того же WSL-дистрибутива, где работает k3s;
- не менее 8 ГиБ свободной RAM и 50 ГиБ диска для полного профиля с
  observability.

## Локальный релизный деплой из WSL (сохранённый сценарий)

Этот раздел остаётся основным для локального релизного деплоя в k3s и не
заменяется CI-пайплайном.

Перейти в каталог проекта на диске Windows и выполнить:

```bash
cd /mnt/c/NAV/Cursor/NewA
chmod +x deploy/k3s/deploy.sh
./deploy/k3s/deploy.sh
```

Скрипт:

1. собирает `nav-saas:dev`;
2. импортирует образ в containerd k3s без внешнего registry;
3. генерирует случайные пароли PostgreSQL/RabbitMQ и JWT secret;
4. применяет манифесты;
5. ждёт готовности приложения и observability-компонентов.

Если Unix-сокет containerd недоступен текущему пользователю, скрипт монтирует
локальный бинарник k3s и сокет в одноразовый контейнер собранного приложения.
Пароль `sudo` и загрузка дополнительного образа для этого не нужны.

Повторный запуск сохраняет данные в PVC и повторно использует существующий
`nav-secrets`. Для переноса в другой кластер или восстановления после удаления
namespace секреты следует сохранить в менеджере секретов и передать явно:

```bash
export POSTGRES_PASSWORD='...'
export RABBITMQ_DEFAULT_USER='nav'
export RABBITMQ_DEFAULT_PASS='...'
export APP_JWT_SECRET='...'
./deploy/k3s/deploy.sh
```

Посмотреть текущие значения можно с правами администратора кластера:

```bash
sudo k3s kubectl -n nav get secret nav-secrets -o yaml
```

Ротация пароля PostgreSQL с согласованным обновлением Kubernetes Secret:

```bash
./deploy/k3s/rotate-postgres-password.sh
```

## Доступ

Узнать адрес узла:

```bash
hostname -I
sudo k3s kubectl get ingress -n nav
```

Если Windows напрямую видит IP узла WSL, добавить строку в файл
`C:\Windows\System32\drivers\etc\hosts`:

```text
<IP_WSL_ИЛИ_УЗЛА_K3S> nav.local
```

После этого открыть `http://nav.local`. Для быстрой проверки без изменения
hosts:

```bash
curl -H 'Host: nav.local' http://127.0.0.1/api/healthz
```

Если localhost forwarding WSL отключён, запустить прокси через Docker Desktop:

```bash
# Локальный релизный контур (namespace nav, Host: nav.local)
./deploy/k3s/start-windows-proxy.sh

# CI staging (namespace newa-staging, Host: newa-staging.local)
./deploy/k3s/start-windows-proxy.sh newa-staging.local
```

Из Windows PowerShell (Docker Desktop):

```powershell
.\deploy\k3s\start-windows-proxy.ps1 -IngressHost newa-staging.local
```

После этого приложение доступно в Windows по `http://localhost:8088` без
изменения `hosts`. После смены IP WSL скрипт нужно запустить повторно.
Одновременно на `:8088` доступен только один Host (локальный или staging).

ProjectStatus разворачивается отдельно из `C:\Codex\ProjectStatus`, но
использует тот же local ingress host `nav.local`. После его `deploy\k3s\deploy.ps1`
и запуска proxy с `-IngressHost nav.local` доступны:

- `http://localhost:8088/projectStatusDesktop/`;
- `http://localhost:8088/projectStatusMobile/`.

Через тот же ingress доступны:

- Grafana: `http://localhost:8088/grafana/`;
- Prometheus: `http://localhost:8088/prometheus/`.

Локальные данные Grafana: `admin` / `admin`. Для общего окружения задайте
`GRAFANA_ADMIN_PASSWORD` перед первым запуском; значение хранится в Kubernetes
Secret `observability-secrets`.

Дашборд k6: `http://localhost:8088/grafana/d/nav-k6-load/k6-load-test`.
Он показывает данные после нагрузочного запуска с Prometheus remote write:

```bat
set K6_BASE_URL=http://host.docker.internal:8088
set K6_PROMETHEUS_RW_SERVER_URL=http://host.docker.internal:8088/prometheus/api/v1/write
scripts\run-k6-load.bat
```

На пустой базе код создаёт пользователя `admin@example.com` с паролем
`admin123`. Пароль нужно сменить сразу после первого входа.

## Проверка и диагностика

```bash
sudo k3s kubectl -n nav get pods,svc,ingress,pvc
sudo k3s kubectl -n nav logs deployment/nav-api --tail=100
sudo k3s kubectl -n nav logs deployment/nav-auth --tail=100
sudo k3s kubectl -n nav logs deployment/nav-calc-worker --tail=100
sudo k3s kubectl -n nav get pods \
  -l 'app in (prometheus,grafana,loki,tempo,otel-collector,alloy)'
```

Проверка маршрутизации auth:

```bash
curl -i -H 'Host: nav.local' \
  -H 'Content-Type: application/json' \
  --data '{"companyName":"Система","name":"admin@example.com","password":"admin123"}' \
  http://127.0.0.1/api/auth/login
```

## Перенос текущей PostgreSQL

Свежий deployment создаёт пустую GSN-схему. Чтобы перенести локально
импортированный справочник и текущие данные, сначала сделать dump исходной БД:

```bash
pg_dump --format=custom --no-owner --no-acl \
  --dbname='postgres://SOURCE_USER:SOURCE_PASSWORD@SOURCE_HOST:5432/SOURCE_DB' \
  --file=nav.dump
```

Затем скопировать dump в pod и восстановить:

```bash
sudo k3s kubectl -n nav cp nav.dump postgres-0:/tmp/nav.dump
sudo k3s kubectl -n nav exec postgres-0 -- \
  pg_restore --clean --if-exists --no-owner --no-acl \
  --username=nav --dbname=nav /tmp/nav.dump
sudo k3s kubectl -n nav rollout restart \
  deployment/nav-api deployment/nav-auth deployment/nav-calc-worker
```

`--clean` перезаписывает содержимое целевой БД. Перед восстановлением рабочей
БД нужно сделать резервную копию PVC/БД.

## Обновление

После изменения исходников повторить запуск `deploy.sh`. Для нескольких узлов
нужен registry, а в `deploy/k3s/app.yaml` следует заменить
`imagePullPolicy: Never` на `IfNotPresent` или `Always`.

## Observability

Полный observability-профиль входит в `deploy/k3s/kustomization.yaml` и
устанавливается основным `deploy.sh`. Если NAV уже развёрнут и пересобирать его
образ не требуется:

```bash
chmod +x deploy/k3s/deploy-observability.sh
./deploy/k3s/deploy-observability.sh
```

Состав:

| Компонент | Назначение | PVC / retention |
|-----------|------------|-----------------|
| Prometheus | scrape `nav-api`, `nav-auth`, `nav-calc-worker`, alert rules | 5 GiB, 7 дней / 4 GB |
| Grafana | datasource provisioning, `NAV Overview` и `k6 Load Test` | 1 GiB |
| Loki | JSON pod logs | 5 GiB, 7 дней |
| Tempo | OTLP traces | 5 GiB, 7 дней |
| OTel Collector | OTLP HTTP/gRPC ingress и batch export в Tempo | без PVC |
| Alloy | чтение NAV pod logs через Kubernetes API и отправка в Loki | без PVC |

Alloy не использует privileged mode и hostPath: один collector читает логи
только из namespace `nav` через Kubernetes API. RBAC ограничен `pods` и
`pods/log`. Loki и Tempo не публикуются через ingress; к ним обращается Grafana
по ClusterIP. Grafana требует login, Prometheus в локальном профиле доступен
без аутентификации — не публикуйте ingress во внешнюю сеть.

NAV получает `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318`, а ссылки
раздела «Мониторинг» ведут на `/grafana/` и `/prometheus/`. Логи остаются JSON
в stdout, поэтому их одновременно видит `kubectl logs` и собирает Alloy.

Prometheus загружает правила:

- недоступность NAV service;
- потеря RabbitMQ connection;
- сообщения в calculation DLQ;
- устаревший heartbeat calc worker;
- рост outbox backlog.

Alertmanager и внешние уведомления пока не включены: правила видны в
Prometheus/Grafana, но email/webhook не отправляются.

Проверка:

```bash
chmod +x deploy/k3s/smoke-observability.sh
./deploy/k3s/smoke-observability.sh
```

Smoke-тест проверяет Ready deployments, три Prometheus target, коррелированные
логи в Loki, traces в Tempo, ingress и provisioning Grafana, health datasource
и загрузку alert rules. Пароль Grafana читается из Secret внутри скрипта и не
выводится.

Конфигурация находится в `deploy/k3s/observability/`. Используются закреплённые
версии из существующего Docker-профиля `deploy/observability/`, чтобы оба
варианта запуска оставались сопоставимыми.

## Связь с GitLab CI/CD

Локальный сценарий выше сохраняется без изменений и нужен для:

- автономного запуска стенда в WSL без внешнего registry;
- диагностики и smoke-проверок до публикации изменений;
- ручного восстановления локального окружения.

Отдельно от этого сценария CI/CD использует staging overlay
`deploy/environments/staging` и deploy по immutable image digest в namespace
`newa-staging`. Это не отменяет и не переписывает локальный профиль `deploy/k3s`.

## Ограничения текущего профиля

- один экземпляр каждого stateful-сервиса и один worker;
- нет TLS и внешнего secret manager;
- observability развёрнут в single-binary/local-storage режиме и не является
  HA; filesystem storage Loki/Tempo подходит только для локального профиля;
- нет Alertmanager и внешних каналов уведомлений;
- `/projectStatusDesktop`, `/projectStatusMobile` и
  `/api/project-status/graphql` не включены: соответствующего сервиса нет в
  этом репозитории;
- начальный пароль администратора зашит в текущем bootstrap-коде.
