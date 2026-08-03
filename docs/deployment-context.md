# Контекст деплоя NewA и ProjectStatus

Состояние зафиксировано **2026-08-03**. Нормативное решение CI/CD:
`docs/ci-cd-decision.md`.

Три контура нельзя смешивать: Windows runtime, локальный k3s, CI staging.

## Текущее состояние

| | |
|--|--|
| Репозиторий GitHub | `C:\NAV\Cursor\NewA` → `https://github.com/AlekseiNadein/NewA` |
| Репозиторий GitLab | `https://gitlab.com/abc-group4363531/NewA` |
| Default branch | `main` (оба remote; `origin/main` ≈ `gitlab/main`) |
| Активный CD | **GitLab CI** (`GITLAB_CD_ENABLED=true`) |
| Publish | BuildKit → GitLab Container Registry (`nav-saas@sha256:…`) |
| Deploy runner | Self-hosted GitLab runner tag `newa-staging` (WSL) |
| Deploy kubeconfig | `/home/alexey/.kube/newa-staging-ci` (SA `newa-ci-deploy`) |
| GitHub Actions | validate/test/security/publish mirror; **deploy/verify `if: false`** |
| `STAGING_BASE_URL` | `http://newa-staging.local` |
| Бэкапы | `backups/2026-08-02_19-09/` (pre-migration), `backups/2026-08-03_11-52/` (cutover) |

### Staging runtime

- NAV LKG: ConfigMap `newa-release-state` после зелёного GitLab deploy/verify.
- ProjectStatus в `newa-staging` (image digest; registry host зависит от promotion).
- Smoke NAV: GitLab CI variables `SMOKE_*`.
- Registry pull: `STAGING_REGISTRY_USER` / `STAGING_REGISTRY_PASSWORD` → secret `newa-registry-pull`.
- Agent Helm release `newa-staging` в ns `gitlab-agent-newa-staging` (установлен; операционный deploy — self-hosted runner).

## Репозитории

- NewA: GitHub + GitLab remotes, default `main`.
- ProjectStatus: `C:\Codex\ProjectStatus`,
  GitHub `AlekseiNadein/ProjectStatus`, GitLab `abc-group4363531/ProjectStatus`.
- ProjectStatus не входит в образ `nav-saas`.

## 1. Windows runtime

Публичный вход — nginx NewA на `http://localhost:8080`.

- auth: `:8081`; NAV API/UI: `:8090`; ProjectStatus: `:8100`;
- `run.bat` поднимает ProjectStatus через `scripts/restart-project-status.bat`;
- маршруты: `/projectStatusDesktop/`, `/projectStatusMobile/`,
  `/api/project-status/graphql`.

## 2. Локальный k3s release

- namespace `nav`, host `nav.local`, image `nav-saas:dev`;
- ProjectStatus: `project-status:dev` из своего репозитория (`deploy/k3s/deploy.ps1`).

Доступ из Windows (один Host на `:8088`):

```powershell
.\deploy\k3s\start-windows-proxy.ps1 -IngressHost nav.local
```

## 3. CI staging

- namespace `newa-staging`, host `newa-staging.local`;
- CD: GitLab pipeline на `main`;
- deploy/rollback: tag `newa-staging`, `resource_group: newa-staging`;
- lean apply: `scripts/deploy-staging.sh` (без observability stack);
- RBAC apply только через `scripts/bootstrap-staging.sh` (admin).

Просмотр staging:

```powershell
.\deploy\k3s\start-windows-proxy.ps1 -IngressHost newa-staging.local
.\deploy\k3s\start-wsl-k3s.ps1
```

## ProjectStatus

Отдельный CI/publish; NewA деплоит в shared staging под тем же lock
`newa-staging`. Promotion file:
`deploy/environments/staging/project-status-image.txt`.

Пока publish ProjectStatus может оставаться на GHCR; целевой путь —
GitLab Registry + promotion MR в NewA. Подробнее:
`docs/project-status-ci-cd.md`, шаблон `docs/templates/project-status.gitlab-ci.yml`.

## Secrets (не коммитить значения)

- `nav-secrets`, `project-status-secrets`;
- GitLab: `SMOKE_*`, `STAGING_REGISTRY_*`, `PROJECT_STATUS_REGISTRY_*`;
- promotion: отдельный `NEWA_PROMOTION_TOKEN`;
- общий `APP_JWT_SECRET` для verify JWT.

## Инварианты

- ProjectStatus read-only к app PostgreSQL; calc только через NAV API.
- Нет записи в Rabbit / `outbox_events` / `estimate_calc_*` из ProjectStatus.
- Сметная стоимость: `calc-status?summary=1 → grandTotal`, не `app_estimates.total`.
- Dual auto-deploy GitHub+GitLab запрещён.

## Остаток работ

1. ProjectStatus: полный GitLab publish + promotion в NewA.
2. Rollback exercise на GitLab.
3. JWKS / durable bulk recalculation (продукт).
4. Hardening `securityContext` (Trivy HIGH report-only).

## Канонические документы

- `docs/ci-cd-decision.md` — итоговое решение CI/CD (ADR).
- `docs/deployment-context.md` — этот файл.
- `docs/gitlab-cicd-setup.md` — операционный GitLab.
- `docs/github-actions-setup.md` — GitHub mirror.
- `.cursor/rules/deployment-contours.mdc` — инварианты для агента.
- `docs/project-status-ci-cd.md` — контракт ProjectStatus staging.
