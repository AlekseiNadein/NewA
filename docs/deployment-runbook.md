# Deployment Runbook (Staging)

## Scope

Deploy path for `main` branch to Kubernetes staging namespace `newa-staging` through GitLab CI.

## Prerequisites

- GitLab project has Kubernetes Agent configured and available context in `KUBE_CONTEXT`.
- Protected CI variables are configured:
  - `KUBE_CONTEXT`
  - `STAGING_REGISTRY_USER`
  - `STAGING_REGISTRY_PASSWORD`
  - optional `STAGING_BASE_URL` for HTTP smoke health check
- Registry publish credentials for CI are available via standard GitLab variables.

## Pipeline flow

1. `validate:*` jobs verify Go code and kustomize rendering.
2. `test:go-unit` runs unit tests.
3. `build:affected` determines impacted components.
4. `publish:image` builds and pushes immutable image tagged by commit SHA and records digest.
5. `deploy:staging` applies manifests with digest-pinned image and waits for rollout.
6. `verify-staging.sh` confirms rollout and optional health endpoint.
7. `resolve-last-known-good.sh` records candidate as last known good metadata artifact.

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
