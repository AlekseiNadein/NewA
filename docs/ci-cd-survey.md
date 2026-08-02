# NewA CI/CD Survey (Stage 0)

## Repository and stack

- Repository model: monorepo.
- Main runtime component: `nav-saas` image from root `Dockerfile`.
- Languages: Go (`backend/**`) and browser JS (`web/**`).
- Package managers and locks: Go modules (`go.mod`, `go.sum`).
- Deploy manifests: `deploy/k3s/**` (Kustomize-based), plus observability manifests.
- Existing GitLab CI config before this change: absent.

## Components and dependency graph

| Component | Built image | Depends on | Rebuild trigger |
|---|---|---|---|
| `nav-api` | `nav-saas` | `backend/**`, `web/**`, `db/**`, `go.mod`, `go.sum`, `Dockerfile` | any listed dependency change |
| `nav-auth` | `nav-saas` | `backend/**`, `web/**`, `db/**`, `go.mod`, `go.sum`, `Dockerfile` | any listed dependency change |
| `nav-calc-worker` | `nav-saas` | `backend/**`, `web/**`, `db/**`, `go.mod`, `go.sum`, `Dockerfile` | any listed dependency change |

Current architecture uses one image for all three workloads with different entrypoints.

## Runtime notes (from manifests)

- Kubernetes target: k3s.
- Current base namespace in manifests: `nav`.
- Staging namespace introduced for CI/CD: `newa-staging`.
- Deploy style: Kustomize apply + rollout checks.
- Update model: rolling updates via Deployment strategy in staging overlay.

## Outstanding owner decisions

Before enforcing strict production-grade gates, owners should finalize:

1. vulnerability policy (Critical/High block thresholds and exception process);
2. source and rotation process for deploy and registry pull secrets;
3. approved smoke-test list for release gates;
4. retention and protection policy for active and last-known-good images;
5. rollback ownership and SLA targets for staging incidents.

Current temporary GitHub gate blocks only Critical Trivy config findings.
High findings from the existing local k3s profile are reported but non-blocking
until hardening of StatefulSet securityContext is completed.

## Decision note: dual deploy contour

The project intentionally keeps two deployment contours with different goals:

- local release contour (`deploy/k3s`, namespace `nav`) for standalone WSL-based
  deployment, diagnostics, and quick environment restore without external
  registry dependency;
- CI staging contour (`deploy/environments/staging`, namespace `newa-staging`)
  for immutable digest-based deployment from GitLab pipeline with rollout/verify
  gates.

These contours are complementary and should not override each other.

## Decision note: temporary GitHub platform

Until cutover, GitHub Actions is the **active** CI/CD executor and GHCR is the
image registry. A self-hosted Linux runner inside the k3s network replaces the
GitLab Agent transport for staging jobs.

GitLab CI is being restored on `feature/gitlab-cicd-migration` with
`GITLAB_CD_ENABLED=false`, so GitLab may shadow-build/publish but must not
auto-deploy while GitHub still deploys. See `docs/gitlab-cicd-setup.md`.

The temporary runner must use namespace-scoped Kubernetes credentials and must
never execute untrusted pull-request code.
