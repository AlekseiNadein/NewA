# Контекст деплоя NewA и ProjectStatus

Состояние зафиксировано **2026-08-02**. Документ описывает три разных контура,
которые нельзя смешивать: Windows runtime, локальный k3s и CI staging.

## Текущее состояние `origin/main`

| | |
|--|--|
| Репозиторий | `C:\NAV\Cursor\NewA` → `https://github.com/AlekseiNadein/NewA` |
| Default branch | `main` |
| Активный CD | GitHub Actions + GHCR (до cutover на GitLab) |
| Миграция на GitLab | ветка `feature/gitlab-cicd-migration`; CD gated `GITLAB_CD_ENABLED=false` |
| GitLab NewA | `https://gitlab.com/abc-group4363531/NewA` |
| GitLab ProjectStatus | `https://gitlab.com/abc-group4363531/ProjectStatus` (создаём) |
| `KUBE_CONTEXT` | `abc-group4363531/NewA:newa-staging` |
| `STAGING_BASE_URL` | тот же: `http://newa-staging.local` |
| Цель | полный cutover на GitLab |
| Бэкап pre-migration | `backups/2026-08-02_19-09/` (sources + `cicd-snapshot`) |

### Уже в `origin/main`

- GitHub Actions CI/CD для NAV staging (`newa-staging`).
- Staging-интеграция ProjectStatus (workflows, manifests, scripts, docs).
- Windows proxy helpers, dual-contour rules.
- UI import пользовательских позиций из смет.
- Замены ресурсов РМ/РС из source-data при расчёте.

### Staging runtime (проверено)

- NAV LKG ConfigMap `newa-release-state` обновляется после зелёного verify.
- ProjectStatus развёрнут в `newa-staging` с immutable GHCR digest;
  LKG в ConfigMap `project-status-release-state`.
- Smoke NAV: Environment secrets `SMOKE_*`.
- ProjectStatus secrets: `project-status-secrets`,
  `project-status-registry-pull`, Environment `PROJECT_STATUS_GHCR_*`.

## Миграция GitLab (в работе)

Целевая платформа — GitLab.com + GitLab Container Registry + GitLab Agent.
Пока `GITLAB_CD_ENABLED` не `true`, GitLab **не** деплоит в staging.
Нельзя одновременно держать auto-deploy на GitHub и GitLab.

Подробности: `docs/gitlab-cicd-setup.md`.

## Репозитории

- NewA: `C:\NAV\Cursor\NewA`,
  `https://github.com/AlekseiNadein/NewA`, default branch `main`.
- ProjectStatus: `C:\Codex\ProjectStatus`,
  `https://github.com/AlekseiNadein/ProjectStatus`, ветка `master`.
- ProjectStatus не включается в образ `nav-saas` и имеет самостоятельный
  Dockerfile, CI и GHCR package.

## 1. Windows runtime

Публичный вход — nginx NewA на `http://localhost:8080`.

- auth: `:8081`;
- NAV API/UI: `:8090`;
- ProjectStatus BFF/UI: `:8100`;
- `run.bat` запускает ProjectStatus через
  `scripts/restart-project-status.bat`;
- маршруты ProjectStatus:
  `/projectStatusDesktop/`, `/projectStatusMobile/`,
  `/api/project-status/graphql`.

Если процесс `project-status.exe` на `:8100` не работает, nginx возвращает 502.

## 2. Локальный k3s release

- namespace: `nav`;
- ingress host: `nav.local`;
- NAV image: `nav-saas:dev`, импортируется в containerd;
- ProjectStatus image: `project-status:dev`, разворачивается из отдельного
  репозитория командой:

```powershell
cd C:\Codex\ProjectStatus
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\k3s\deploy.ps1
```

ProjectStatus использует Service `project-status:8100`, общую app PostgreSQL,
NAV JWT и внутренний URL `http://nav-api:8090`.

### Доступ из Windows

Docker container `nav-k3s-proxy` публикует `127.0.0.1:8088` и подставляет Host
для Traefik:

```powershell
cd C:\NAV\Cursor\NewA
.\deploy\k3s\start-windows-proxy.ps1 -IngressHost nav.local
```

Проверенные адреса:

- `http://localhost:8088/projectStatusDesktop/`;
- `http://localhost:8088/projectStatusMobile/`.

На `:8088` одновременно маршрутизируется только один ingress host.

## 3. CI staging

- namespace: `newa-staging`;
- ingress host внутри WSL: `newa-staging.local`;
- GitHub Actions + GHCR — **активный** CD до cutover;
- GitLab CI обновляется в `feature/gitlab-cicd-migration`, но deploy
  выключен (`GITLAB_CD_ENABLED=false`);
- self-hosted GitHub runner:
  `newa-staging-nadein-envyi5`, labels
  `self-hosted`, `Linux`, `newa-staging`;
- kubeconfig runner:
  `/home/alexey/.kube/newa-staging-ci`;
- runner имеет namespace-scoped RBAC, не cluster-admin;
- NAV deploy использует immutable `nav-saas@sha256:...`;
- last-known-good NAV хранится в ConfigMap `newa-release-state`.
- целевой транспорт GitLab: Agent `.gitlab/agents/newa-staging`.

Первый зелёный полный NAV staging run (исторически):
`https://github.com/AlekseiNadein/NewA/actions/runs/30540393140`.

Зелёный run после PR #2:
`https://github.com/AlekseiNadein/NewA/actions/runs/30628927056`.

Windows proxy для просмотра staging переключается явно:

```powershell
.\deploy\k3s\start-windows-proxy.ps1 -IngressHost newa-staging.local
```

После этого `http://localhost:8088/` показывает staging, а не локальный `nav`.

WSL/runner keepalive:

```powershell
.\deploy\k3s\start-wsl-k3s.ps1
```

## ProjectStatus CI/CD

ProjectStatus workflow:

1. проверяет форматирование, `go vet`, gqlgen drift и k3s manifests;
2. запускает `go test ./...`;
3. выполняет Gitleaks и Trivy;
4. публикует
   `ghcr.io/alekseinadein/projectstatus/project-status@sha256:...`;
5. при наличии `NEWA_PROMOTION_TOKEN` открывает promotion PR в NewA.

Первый зелёный полный run:
`https://github.com/AlekseiNadein/ProjectStatus/actions/runs/30611794492`,
commit `4e63476`.

В ProjectStatus настроена variable:
`NEWA_REPOSITORY=AlekseiNadein/NewA`.
`NEWA_PROMOTION_TOKEN` пока не настроен, поэтому promotion сообщается как
notice и пропускается; образ при этом публикуется успешно.

## Staging-интеграция ProjectStatus в NewA

Уже в `origin/main` и применено в кластере:

- `.github/workflows/deploy-project-status-staging.yml`;
- `.github/workflows/rollback-project-status-staging.yml`;
- `deploy/environments/staging/project-status.yaml`;
- `scripts/deploy-project-status-staging.sh`;
- `scripts/smoke-project-status-staging.sh`;
- `scripts/record-project-status-release.sh`;
- `scripts/provision-project-status-staging.sh`;
- `docs/project-status-ci-cd.md`.

Целевая схема:

```text
ProjectStatus main
  -> test/security
  -> GHCR image@digest
  -> promotion PR в NewA
  -> NewA environment workflow
  -> deployment/project-status в newa-staging
  -> smoke
  -> ConfigMap project-status-release-state
```

NAV и ProjectStatus deploy workflows используют общий concurrency group
`newa-staging`, чтобы два репозитория не меняли namespace одновременно.

## Secrets и доступ к БД

Не хранить значения секретов в документации или репозитории.

- NewA staging: `nav-secrets`;
- ProjectStatus staging: отдельный `project-status-secrets`;
- `scripts/provision-project-status-staging.sh` создаёт роль
  `project_status_ro` с SELECT только на contract tables;
- GHCR pull: `PROJECT_STATUS_GHCR_USERNAME`,
  `PROJECT_STATUS_GHCR_TOKEN`;
- promotion: отдельный ограниченный `NEWA_PROMOTION_TOKEN`;
- общий HMAC `APP_JWT_SECRET` пока нужен для verify JWT; browser его не получает.
- NAV staging smoke: `SMOKE_*`, pull через `GITHUB_TOKEN` / optional username.

## Инварианты интеграции

- ProjectStatus читает app PostgreSQL, но не пишет в неё.
- Расчёт запускается только через NAV API.
- Нельзя писать в RabbitMQ, `outbox_events`, `estimate_calc_jobs` или
  `estimate_calc_*` из ProjectStatus.
- Стоимость сметы берётся из `calc-status?summary=1 -> grandTotal`, не из
  `app_estimates.total`.
- Изменения схемы NewA выполняются через expand/migrate/contract.
- Изменения auth/login/JWT/calc tables требуют проверки ProjectStatus.

## Что остаётся сделать

1. Настроить отдельный `NEWA_PROMOTION_TOKEN` в ProjectStatus для
   автоматических promotion PR.
2. Выполнить rollback exercise (`Rollback staging` с `exercise=true` и/или
   `Rollback ProjectStatus staging` после появления второго digest).
3. Отдельно спроектировать JWKS/public-key verify и durable bulk recalculation.
4. Hardening k3s `securityContext` (Trivy HIGH пока report-only).

## Канонические документы

- `docs/deployment-context.md` — этот файл, актуальный снимок состояния.
- `docs/gitlab-cicd-setup.md` — целевой GitLab CI/CD и cutover.
- `.cursor/rules/deployment-contours.mdc` — краткие инварианты для агента.
- `deploy/k3s/README.md` — локальный k3s NewA и Windows proxy.
- `docs/github-actions-setup.md` — временный GitHub path (active until cutover).
- `docs/project-status-ci-cd.md` — staging-интеграция ProjectStatus.
- `RESOURCE_ANALYTICS_SERVICE_CONTEXT.md` — data/auth/calc contract.
- `C:\Codex\ProjectStatus\docs\ci-cd.md` — CI отдельного репозитория.
- `C:\Codex\ProjectStatus\docs\decisions.md` — ADR ProjectStatus.
