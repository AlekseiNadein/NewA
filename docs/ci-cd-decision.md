# ADR: итоговое решение CI/CD NewA

Дата фиксации: **2026-08-03**.  
Статус: **принято и внедрено**.

## Решение

**Активный staging CD — GitLab.com.**  
GitHub Actions остаётся зеркалом validate/test/security/publish и **не** деплоит в `newa-staging`.

| Область | Решение |
|---|---|
| Git-платформа CD | GitLab.com `abc-group4363531/NewA` |
| Зеркало / legacy git | GitHub `AlekseiNadein/NewA` (без auto-deploy) |
| Registry (NAV) | GitLab Container Registry, immutable `@sha256:…` |
| Publish | BuildKit rootless на shared runners |
| Deploy/verify/rollback | Self-hosted GitLab runner tag `newa-staging` в WSL |
| Kube access на deploy | `KUBECONFIG=/home/alexey/.kube/newa-staging-ci` (SA `newa-ci-deploy`) |
| Agent | Зарегистрирован (`newa-staging`); для smoke DNS используется host runner |
| Namespace / host | `newa-staging` / `newa-staging.local` |
| Lock | `resource_group: newa-staging` на весь apply→smoke→LKG |
| LKG | ConfigMap `newa-release-state` |
| CD flag | `GITLAB_CD_ENABLED=true` (project variable + default в workflow) |
| Manifest strategy | Lean `scripts/deploy-staging.sh` (не полный kustomize+observability) |
| RBAC | Namespace-scoped; roles/rolebindings только read; apply RBAC — bootstrap |
| ProjectStatus | Отдельный GitLab проект `abc-group4363531/ProjectStatus`; NewA владеет deploy |

## Три контура (не смешивать)

1. **Windows runtime** — `run.bat`, nginx `:8080`, локальные процессы.
2. **Local k3s** — ns `nav`, host `nav.local`, images `*:dev`.
3. **CI staging** — ns `newa-staging`, host `newa-staging.local`, digest-only.

Windows proxy `:8088` обслуживает **один** Ingress Host за раз.

## Pipeline

```text
MR / branch     → validate, test, scan
main            → build:affected, publish:image (GitLab Registry)
main + CD on    → deploy:staging on tag newa-staging
                  (registry pull secret → lean apply → rollout →
                   verify → write-smoke → RBAC assert → LKG)
manual          → rollback:staging / exercise / project-status
```

## Почему self-hosted runner для deploy

Shared GitLab runners не резолвят `newa-staging.local` и не имеют
стабильного пути к private k3s API для smoke. Deploy/rollback jobs
помечены `tags: [newa-staging]` и используют уже проверенный
namespace-scoped kubeconfig на WSL-хосте рядом с кластером.

GitLab Agent остаётся частью целевой модели и установлен в кластере;
операционный deploy path после cutover — self-hosted runner + kubeconfig.

## Что явно не делаем

- Одновременный auto-deploy GitHub + GitLab.
- `cluster-admin` для CI identity.
- Создание namespace из deploy job.
- Полный staging kustomize с observability в CD path.
- Хранение секретов в git.

## ProjectStatus

| Сейчас | Дальше |
|---|---|
| Publish ещё может идти через GHCR | Перевести publish на GitLab Registry |
| Deploy в NewA через GitLab job / promotion file | Тот же `resource_group: newa-staging` |
| GitLab project создан | Довести `.gitlab-ci.yml` и promotion MR |

## Операционные якоря

- GitLab NewA: https://gitlab.com/abc-group4363531/NewA
- GitLab ProjectStatus: https://gitlab.com/abc-group4363531/ProjectStatus
- `STAGING_BASE_URL=http://newa-staging.local`
- Runner keepalive: `deploy/k3s/start-wsl-k3s.ps1`
- Proxy staging: `start-windows-proxy.ps1 -IngressHost newa-staging.local`
- Pre-migration backup: `backups/2026-08-02_19-09/`
- Cutover backup: `backups/2026-08-03_11-52/`

## Канонические документы

- Этот ADR — нормативное решение.
- `docs/deployment-context.md` — живой снимок контуров.
- `docs/gitlab-cicd-setup.md` — операционная настройка GitLab.
- `docs/github-actions-setup.md` — GitHub как mirror (без CD).
- `.cursor/rules/deployment-contours.mdc` — краткие инварианты для агента.
