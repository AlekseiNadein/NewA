# Deployment Runbook (Staging)

## Scope

Deploy path for `main` branch to Kubernetes staging namespace `newa-staging`.
GitHub Actions is the temporary active CI/CD platform; GitLab configuration is
retained for future use.
This runbook does not replace local k3s release flow from `deploy/k3s/README.md`.

## Prerequisites

- GitHub self-hosted runner with labels `linux` and `newa-staging` is registered
  inside the k3s network.
- GitHub Environment `staging`, variables and secrets are configured according
  to `docs/github-actions-setup.md`.
- Runner kubeconfig uses a namespace-scoped identity, not `cluster-admin`.

## Pipeline flow

1. `validate`, `test` and `security` run on GitHub-hosted runners.
2. `publish` builds and pushes an immutable SHA-tagged image to GHCR.
3. `deploy` applies the digest-pinned image and waits for rollout.
4. `verify` runs health, write-smoke and RBAC negative checks.
5. `resolve-last-known-good.sh` records the verified digest in the cluster and
   uploads release evidence.

## Manual checks after deploy

- `kubectl -n newa-staging get pods`
- `kubectl -n newa-staging get deploy -o wide`
- Open staging URL and verify login + core API scenario.

## Evidence to store

- Pipeline ID and commit SHA.
- Published image tag and digest.
- Rollout status output.
- Smoke check results.
- Produced `out/last-known-good.env` artifact.
