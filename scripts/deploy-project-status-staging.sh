#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
KUBECTL="${KUBECTL:-kubectl}"
IMAGE="${PROJECT_STATUS_IMAGE:?PROJECT_STATUS_IMAGE must be image@sha256:digest}"

if [[ ! "${IMAGE}" =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]]; then
  echo "PROJECT_STATUS_IMAGE must be an immutable registry image@sha256:digest" >&2
  exit 1
fi

for secret in project-status-secrets project-status-registry-pull; do
  if ! "${KUBECTL}" -n "${NAMESPACE}" get secret "${secret}" >/dev/null 2>&1; then
    echo "Required secret ${NAMESPACE}/${secret} does not exist" >&2
    exit 1
  fi
done

sed "s|PROJECT_STATUS_IMAGE|${IMAGE}|g" \
  "${ROOT_DIR}/deploy/environments/staging/project-status.yaml" |
  "${KUBECTL}" apply -f -

"${KUBECTL}" -n "${NAMESPACE}" rollout status \
  deployment/project-status --timeout="${ROLLOUT_TIMEOUT:-300s}"

echo "ProjectStatus deployed to ${NAMESPACE}: ${IMAGE}"
