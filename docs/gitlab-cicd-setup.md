# GitLab CI/CD setup (active platform)

Нормативное решение: `docs/ci-cd-decision.md`.  
GitLab.com — **активный** CD для `newa-staging`. GitHub не auto-деплоит.

## Итоговое состояние

| Piece | Status |
|---|---|
| GitLab NewA | `https://gitlab.com/abc-group4363531/NewA` |
| GitLab ProjectStatus | `https://gitlab.com/abc-group4363531/ProjectStatus` |
| `GITLAB_CD_ENABLED` | **`true`** |
| Publish | BuildKit rootless → GitLab Registry |
| Deploy/rollback | Self-hosted runner tag **`newa-staging`** |
| Kubeconfig on runner | `/home/alexey/.kube/newa-staging-ci` |
| Agent | Installed (`newa-staging`); smoke/DNS via host runner |
| Lean deploy | `scripts/deploy-staging.sh` |
| Lock | `resource_group: newa-staging` |
| LKG | ConfigMap `newa-release-state` |
| `STAGING_BASE_URL` | `http://newa-staging.local` |
| GitHub deploy/verify | Disabled (`if: false`) |

Backups: `backups/2026-08-02_19-09/`, `backups/2026-08-03_11-52/`.

## CI/CD variables

| Type | Key | Notes |
|---|---|---|
| Variable | `GITLAB_CD_ENABLED` | `true` |
| Variable | `STAGING_BASE_URL` | `http://newa-staging.local` |
| Variable | `STAGING_NAMESPACE` | `newa-staging` |
| Variable | `KUBE_CONTEXT` | `abc-group4363531/NewA:newa-staging` (Agent; optional for host path) |
| Masked | `STAGING_REGISTRY_USER` / `STAGING_REGISTRY_PASSWORD` | GitLab Registry pull |
| Masked | `SMOKE_COMPANY_NAME` / `SMOKE_USER_NAME` / `SMOKE_PASSWORD` | write-smoke |
| Masked | `PROJECT_STATUS_REGISTRY_USER` / `PROJECT_STATUS_REGISTRY_PASSWORD` | ProjectStatus pull |

Project settings: protect `main`, MR-only merge, newest-ready-first,
prevent outdated deployment jobs, Environment `staging`.

## Pipeline shape

```text
MR / branch  → validate, test, scan
main         → build:affected, publish:image
main + CD    → deploy:staging (tag newa-staging): apply→smoke→LKG
manual       → rollback:*
```

Deploy **не** создаёт namespace. Bootstrap RBAC/ns:
`scripts/bootstrap-staging.sh` с admin kubeconfig.

## Self-hosted runner

- Tag: `newa-staging`
- Host: WSL рядом с k3s (резолв `newa-staging.local`, доступ к API)
- Keepalive: `deploy/k3s/start-wsl-k3s.ps1`
- Тот же контур, что ранее использовался GitHub Actions runner’ом

## Cutover (выполнен)

1. Shadow CI зелёный, CD off.
2. Agent зарегистрирован; RBAC сужен (KSV-0050).
3. GitHub deploy/verify выключены.
4. `GITLAB_CD_ENABLED=true`.
5. Deploy переведён на self-hosted tag `newa-staging` (smoke DNS).

Осталось: ProjectStatus publish на GitLab Registry + promotion; rollback exercise.

## Related docs

- `docs/ci-cd-decision.md` — ADR
- `docs/deployment-context.md` — живой снимок
- `docs/github-actions-setup.md` — mirror path
- `docs/project-status-ci-cd.md`
