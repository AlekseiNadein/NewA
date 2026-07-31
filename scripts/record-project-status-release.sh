#!/usr/bin/env bash
set -Eeuo pipefail

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
IMAGE="${PROJECT_STATUS_IMAGE:?PROJECT_STATUS_IMAGE is required}"
COMMIT="${PROJECT_STATUS_COMMIT:-}"
PIPELINE="${GITHUB_RUN_ID:-${CI_PIPELINE_ID:-}}"
RECORDED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

kubectl -n "${NAMESPACE}" create configmap project-status-release-state \
  --from-literal=image="${IMAGE}" \
  --from-literal=commit="${COMMIT}" \
  --from-literal=pipeline="${PIPELINE}" \
  --from-literal=recordedAt="${RECORDED_AT}" \
  --dry-run=client -o yaml |
  kubectl apply -f -

echo "Recorded ProjectStatus last-known-good image: ${IMAGE}"
