#!/usr/bin/env sh
set -eu

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
TIMEOUT="${ROLLOUT_TIMEOUT:-300s}"

kubectl -n "${NAMESPACE}" rollout status deployment/nav-auth --timeout="${TIMEOUT}"
kubectl -n "${NAMESPACE}" rollout status deployment/nav-calc-worker --timeout="${TIMEOUT}"
kubectl -n "${NAMESPACE}" rollout status deployment/nav-api --timeout="${TIMEOUT}"

kubectl -n "${NAMESPACE}" get pods

if [ -n "${STAGING_BASE_URL:-}" ]; then
  HEALTH_URL="${STAGING_BASE_URL%/}/api/healthz"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error "${HEALTH_URL}" > /dev/null
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "${HEALTH_URL}" > /dev/null
  else
    echo "Neither curl nor wget is available for health check" >&2
    exit 1
  fi
fi

echo "Staging verification completed."
