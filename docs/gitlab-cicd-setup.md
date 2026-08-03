# GitLab CI/CD setup (target platform)

GitLab.com is the target CI/CD platform for NewA. GitHub Actions + GHCR remain
the **active** staging deployer until cutover. Dual auto-deploy must never be
enabled.

Backup of the pre-migration GitHub solution:
`backups/2026-08-02_19-09/cicd-snapshot/`.

## Current migration state

| Piece | Status |
|---|---|
| GitLab NewA | `https://gitlab.com/abc-group4363531/NewA` |
| GitLab ProjectStatus | `https://gitlab.com/abc-group4363531/ProjectStatus` (to create) |
| Agent `KUBE_CONTEXT` | `abc-group4363531/NewA:newa-staging` |
| `.gitlab-ci.yml` + `.gitlab/ci/*` | Updated to match GitHub behavior |
| BuildKit rootless → GitLab Registry | Ready (shadow publish OK) |
| Lean deploy via `scripts/deploy-staging.sh` | Ready |
| Single `resource_group: newa-staging` job | Ready (apply→smoke→LKG) |
| GitLab Agent config | `.gitlab/agents/newa-staging/config.yaml` |
| `GITLAB_CD_ENABLED` | **`false`** until GitHub CD disabled |
| `STAGING_BASE_URL` | Same as GitHub: `http://newa-staging.local` |
| GitHub Actions | Still active; disable only at cutover |
| Target | Full cutover to GitLab (not dual forever) |

## Required GitLab project settings

1. Project already exists: `abc-group4363531/NewA`.
2. Create sibling project `abc-group4363531/ProjectStatus`.
3. Protect `main`; allow merge only via MR.
4. CI/CD → General pipelines:
   - process mode **newest ready first**;
   - enable **prevent outdated deployment jobs**.
5. Create Environment `staging`.
6. Register GitLab Agent for Kubernetes from
   `.gitlab/agents/newa-staging/config.yaml`, then set:

| Type | Key | Notes |
|---|---|---|
| Variable | `KUBE_CONTEXT` | `abc-group4363531/NewA:newa-staging` |
| Variable | `STAGING_BASE_URL` | `http://newa-staging.local` (same as GitHub) |
| Variable | `GITLAB_CD_ENABLED` | Keep `false` until cutover |
| Variable | `STAGING_NAMESPACE` | `newa-staging` (default in workflow) |
| Masked | `STAGING_REGISTRY_USER` | Pull identity for GitLab Registry |
| Masked | `STAGING_REGISTRY_PASSWORD` | Deploy-token / project token read |
| Masked | `SMOKE_COMPANY_NAME` | NAV write-smoke (copy from GitHub) |
| Masked | `SMOKE_USER_NAME` | NAV write-smoke |
| Masked | `SMOKE_PASSWORD` | NAV write-smoke |
| Masked | `PROJECT_STATUS_REGISTRY_USER` | ProjectStatus image pull |
| Masked | `PROJECT_STATUS_REGISTRY_PASSWORD` | ProjectStatus image pull |

Note: GitLab group path is `abc-group4363531` (from the project URL). The shorter
`abc-group/newa` form is not the live path unless the group/project is renamed.

## Pipeline shape

```text
MR / branch  → validate, test, scan
main         → + build:affected, publish:image (GitLab Registry digest)
main + GITLAB_CD_ENABLED=true
             → deploy:staging (lean stack, smoke, LKG ConfigMap)
             → deploy:project-status:staging (promotion file / manual)
manual       → rollback:staging / rollback:exercise / rollback:project-status
```

Deploy uses namespace-scoped Agent access. Jobs must **not** create the
namespace. Bootstrap remains `scripts/bootstrap-staging.sh` with an admin
kubeconfig.

## Cutover checklist

1. Shadow: GitLab validate/test/scan/publish green on `main`; CD still off.
2. Install/connect Agent; confirm `kubectl` context from a manual job.
3. **Disable GitHub staging CD first** (workflow `if: false` or pause Environment).
4. Set `GITLAB_CD_ENABLED=true`.
5. Run one NAV deploy + smoke; confirm `newa-release-state`.
6. Migrate ProjectStatus publish/promotion to GitLab Registry + MR.
7. Archive or disable GitHub Actions workflows after a stable soak.

## Local contours unchanged

- Windows runtime (`run.bat`, nginx `:8080`) — untouched.
- Local k3s `nav` / `nav.local` — still `deploy/k3s/deploy.sh`.
- CI staging `newa-staging` / `newa-staging.local` — digest deploys only.

Windows proxy still serves one Host at a time on `:8088`.

## Related docs

- `docs/deployment-context.md` — live contour snapshot
- `docs/github-actions-setup.md` — temporary GitHub path (active until cutover)
- `docs/project-status-ci-cd.md` — ProjectStatus promotion contract
- `docs/ci-cd-implementation-guide.md` — original GitLab target design
