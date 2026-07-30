# Temporary GitHub Actions CI/CD

GitHub Actions and GHCR temporarily replace GitLab CI and GitLab Container
Registry. Existing `.gitlab-ci.yml` files remain in the repository for a later
return to GitLab.

## GitHub interface

After pushing the repository to GitHub:

- `Actions` shows CI/CD and rollback workflows;
- `Settings → Secrets and variables → Actions` stores CI/CD configuration;
- `Settings → Environments → staging` configures protection and approvals;
- `Settings → Actions → Runners` registers the local staging runner;
- `Packages` on the repository/owner page shows the `nav-saas` OCI image.

## Required repository settings

Create GitHub Environment `staging`. Add:

Variables:

- `KUBE_CONTEXT` — optional explicit context available on the self-hosted runner;
- `STAGING_BASE_URL` — externally reachable staging URL.

Secrets:

- `GHCR_PULL_USERNAME`;
- `GHCR_PULL_TOKEN` — classic PAT with only `read:packages`;
- `SMOKE_COMPANY_NAME`;
- `SMOKE_USER_NAME`;
- `SMOKE_PASSWORD`.

Current repository `AlekseiNadein/NewA` already has Environment `staging` with these
values configured for the temporary GitHub path.

The workflow's standard `GITHUB_TOKEN` publishes images to GHCR. Repository
workflow permissions must allow package writes.

## Self-hosted runner

Register a Linux runner in the WSL environment that can reach k3s. Assign custom
label `newa-staging`; workflows also require labels `self-hosted` and `linux`.

Current runner:

- repository: `AlekseiNadein/NewA`;
- name: `newa-staging-nadein-envyi5`;
- installation: `/opt/actions-runner`;
- service: `actions.runner.AlekseiNadein-NewA.newa-staging-nadein-envyi5.service`.

WSL must remain running for the runner to stay online:

```powershell
.\deploy\k3s\start-wsl-k3s.ps1
```

Reproducible registration helper:

```bash
bash scripts/install-github-runner.sh AlekseiNadein/NewA newa-staging-nadein-envyi5
```

Required runner tools:

- `kubectl`;
- `curl`;
- `jq`;
- POSIX shell;
- a kubeconfig for a namespace-scoped deploy identity.

Do not enable this runner for workflows from forks or untrusted pull requests.
The current workflow sends only `main` deploy/verify jobs to it; PR validation
runs on GitHub-hosted runners.

## First staging bootstrap

Before the first automated deploy, an administrator must:

1. create namespace `newa-staging`;
2. provision staging runtime secrets (`nav-secrets`,
   `observability-secrets`);
3. provision databases, broker, storage and ingress from the existing k3s
   manifests;
4. configure a namespace-scoped runner identity;
5. verify GHCR pull access.

The CI workflow must not generate application database/JWT secrets.

## Last known good

After successful rollout, health, write-smoke and RBAC checks, the workflow
writes ConfigMap `newa-release-state` in `newa-staging`. It stores image digest,
commit, workflow run ID and timestamp. The manual rollback workflow uses this
record when no explicit image is supplied.
