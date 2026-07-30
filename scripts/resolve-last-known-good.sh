#!/usr/bin/env sh
set -eu

mkdir -p out

if [ -z "${IMAGE_REPOSITORY:-}" ] || [ -z "${IMAGE_DIGEST:-}" ]; then
  echo "IMAGE_REPOSITORY and IMAGE_DIGEST are required" >&2
  exit 1
fi

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
IMAGE="${IMAGE_REPOSITORY}@${IMAGE_DIGEST}"
COMMIT="${CI_COMMIT_SHA:-${GITHUB_SHA:-}}"
PIPELINE="${CI_PIPELINE_ID:-${GITHUB_RUN_ID:-}}"
RECORDED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

cat > out/last-known-good.env <<EOF
LAST_KNOWN_GOOD_IMAGE=${IMAGE}
LAST_KNOWN_GOOD_COMMIT=${COMMIT}
LAST_KNOWN_GOOD_PIPELINE=${PIPELINE}
LAST_KNOWN_GOOD_AT=${RECORDED_AT}
EOF

if command -v kubectl >/dev/null 2>&1; then
  kubectl -n "${NAMESPACE}" create configmap newa-release-state \
    --from-literal=image="${IMAGE}" \
    --from-literal=commit="${COMMIT}" \
    --from-literal=pipeline="${PIPELINE}" \
    --from-literal=recordedAt="${RECORDED_AT}" \
    --dry-run=client -o yaml | kubectl apply -f -
fi

echo "Recorded last known good image: ${IMAGE}"
