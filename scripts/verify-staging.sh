#!/usr/bin/env sh
set -eu

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
TIMEOUT="${ROLLOUT_TIMEOUT:-300s}"
# Ingress can return 502 briefly after rollout while the new pod warms up.
HEALTH_TIMEOUT_SEC="${STAGING_HEALTH_TIMEOUT_SEC:-180}"
HEALTH_INTERVAL_SEC="${STAGING_HEALTH_INTERVAL_SEC:-5}"

kubectl -n "${NAMESPACE}" rollout status deployment/nav-auth --timeout="${TIMEOUT}"
kubectl -n "${NAMESPACE}" rollout status deployment/nav-calc-worker --timeout="${TIMEOUT}"
kubectl -n "${NAMESPACE}" rollout status deployment/nav-api --timeout="${TIMEOUT}"

kubectl -n "${NAMESPACE}" get pods

if [ -n "${STAGING_BASE_URL:-}" ]; then
  HEALTH_URL="${STAGING_BASE_URL%/}/api/healthz"
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    echo "Neither curl nor wget is available for health check" >&2
    exit 1
  fi

  echo "Waiting up to ${HEALTH_TIMEOUT_SEC}s for ${HEALTH_URL}"
  started="$(date +%s)"
  attempt=0
  while :; do
    attempt=$((attempt + 1))
    if command -v curl >/dev/null 2>&1; then
      if curl --fail --silent --show-error --max-time 10 "${HEALTH_URL}" > /dev/null; then
        echo "Health check passed on attempt ${attempt}"
        break
      fi
    else
      if wget -qO- --timeout=10 "${HEALTH_URL}" > /dev/null; then
        echo "Health check passed on attempt ${attempt}"
        break
      fi
    fi

    now="$(date +%s)"
    elapsed=$((now - started))
    if [ "${elapsed}" -ge "${HEALTH_TIMEOUT_SEC}" ]; then
      echo "Health check failed after ${elapsed}s (${attempt} attempts): ${HEALTH_URL}" >&2
      exit 1
    fi
    echo "Health check not ready yet (attempt ${attempt}, elapsed ${elapsed}s); retry in ${HEALTH_INTERVAL_SEC}s"
    sleep "${HEALTH_INTERVAL_SEC}"
  done
fi

echo "Staging verification completed."
