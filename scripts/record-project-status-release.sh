#!/usr/bin/env bash
set -Eeuo pipefail

mkdir -p out

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
IMAGE="${PROJECT_STATUS_IMAGE:?PROJECT_STATUS_IMAGE is required}"
COMMIT="${PROJECT_STATUS_COMMIT:-${CI_COMMIT_SHA:-${GITHUB_SHA:-}}}"
PIPELINE="${CI_PIPELINE_ID:-${GITHUB_RUN_ID:-}}"
RECORDED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ ! "${IMAGE}" =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]]; then
  echo "PROJECT_STATUS_IMAGE must be an immutable registry image@sha256:digest" >&2
  exit 1
fi

cat > out/project-status-last-known-good.env <<EOF
PROJECT_STATUS_LAST_KNOWN_GOOD_IMAGE=${IMAGE}
PROJECT_STATUS_LAST_KNOWN_GOOD_COMMIT=${COMMIT}
PROJECT_STATUS_LAST_KNOWN_GOOD_PIPELINE=${PIPELINE}
PROJECT_STATUS_LAST_KNOWN_GOOD_AT=${RECORDED_AT}
EOF

kubectl -n "${NAMESPACE}" create configmap project-status-release-state \
  --from-literal=image="${IMAGE}" \
  --from-literal=commit="${COMMIT}" \
  --from-literal=pipeline="${PIPELINE}" \
  --from-literal=recordedAt="${RECORDED_AT}" \
  --dry-run=client -o yaml |
  kubectl apply -f -

echo "Recorded ProjectStatus last-known-good image: ${IMAGE}"
