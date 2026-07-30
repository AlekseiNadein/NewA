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
