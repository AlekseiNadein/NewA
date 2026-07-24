# NAV в k3s

В каталоге находится одновузловой deployment-профиль для локального k3s:

- `nav-api` — UI и основной API;
- `nav-auth` — auth/account API;
- `nav-calc-worker` — обработчик расчётов;
- PostgreSQL 17 — постоянное состояние приложения и GSN;
- RabbitMQ 4.1 — очередь расчётов;
- Redis 8 — кэш GSN;
- Traefik Ingress — единая точка входа `http://nav.local`.

PostgreSQL, RabbitMQ и Redis используют PVC стандартного storage class k3s
(`local-path`, если конфигурация k3s не менялась). Это профиль для одного узла,
а не HA-конфигурация.

## Требования

- k3s и встроенный Traefik;
- `sudo` без интерактивного запроса для `k3s` либо заранее настроенные
  переменные `KUBECTL` и `K3S`;
- Docker или Podman внутри того же WSL-дистрибутива, где работает k3s;
- не менее 4 ГиБ свободной RAM и 30 ГиБ диска.

## Деплой из WSL

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
5. ждёт готовности всех компонентов.

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

## Доступ

Узнать адрес узла:

```bash
hostname -I
sudo k3s kubectl get ingress -n nav
```

В Windows добавить строку в файл
`C:\Windows\System32\drivers\etc\hosts`:

```text
<IP_WSL_ИЛИ_УЗЛА_K3S> nav.local
```

После этого открыть `http://nav.local`. Для быстрой проверки без изменения
hosts:

```bash
curl -H 'Host: nav.local' http://127.0.0.1/api/healthz
```

На пустой базе код создаёт пользователя `admin@example.com` с паролем
`admin123`. Пароль нужно сменить сразу после первого входа.

## Проверка и диагностика

```bash
sudo k3s kubectl -n nav get pods,svc,ingress,pvc
sudo k3s kubectl -n nav logs deployment/nav-api --tail=100
sudo k3s kubectl -n nav logs deployment/nav-auth --tail=100
sudo k3s kubectl -n nav logs deployment/nav-calc-worker --tail=100
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

## Ограничения текущего профиля

- один экземпляр каждого stateful-сервиса и один worker;
- нет TLS и внешнего secret manager;
- observability-стек из `deploy/observability` не перенесён;
- `/projectStatusDesktop`, `/projectStatusMobile` и
  `/api/project-status/graphql` не включены: соответствующего сервиса нет в
  этом репозитории;
- начальный пароль администратора зашит в текущем bootstrap-коде.
