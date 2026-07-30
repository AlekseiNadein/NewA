#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMESPACE="${NAMESPACE:-nav}"
KUBECTL="${KUBECTL:-k3s kubectl}"

secret_value() {
  local encoded
  encoded="$(${KUBECTL} -n "${NAMESPACE}" get secret observability-secrets \
    -o 'jsonpath={.data.GRAFANA_ADMIN_PASSWORD}' 2>/dev/null || true)"
  if [[ -n "${encoded}" ]]; then
    printf '%s' "${encoded}" | base64 --decode
  fi
}

if ! ${KUBECTL} -n "${NAMESPACE}" get secret nav-secrets >/dev/null 2>&1; then
  echo "nav-secrets is missing. Deploy NAV with deploy/k3s/deploy.sh first." >&2
  exit 1
fi

GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-$(secret_value)}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-admin}"

echo "[1/4] Creating the Grafana secret"
${KUBECTL} -n "${NAMESPACE}" create secret generic observability-secrets \
  --from-literal=GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD}" \
  --dry-run=client -o yaml | ${KUBECTL} apply -f -

observability_existed=0
if ${KUBECTL} -n "${NAMESPACE}" get deployment prometheus >/dev/null 2>&1; then
  observability_existed=1
fi

echo "[2/4] Applying the k3s stack"
${KUBECTL} apply -k "${ROOT_DIR}/deploy/k3s"

echo "[3/4] Starting storage and query backends"
if [[ "${observability_existed}" == "1" ]]; then
  ${KUBECTL} -n "${NAMESPACE}" rollout restart \
    deployment/prometheus \
    deployment/loki \
    deployment/tempo \
    deployment/grafana
fi
for deployment in prometheus loki tempo grafana; do
  ${KUBECTL} -n "${NAMESPACE}" rollout status \
    "deployment/${deployment}" --timeout=600s
done

echo "[4/4] Starting collectors, then telemetry producers"
${KUBECTL} -n "${NAMESPACE}" rollout restart \
  deployment/otel-collector \
  deployment/alloy
for deployment in otel-collector alloy; do
  ${KUBECTL} -n "${NAMESPACE}" rollout status \
    "deployment/${deployment}" --timeout=300s
done

${KUBECTL} -n "${NAMESPACE}" rollout restart \
  deployment/nav-api \
  deployment/nav-auth \
  deployment/nav-calc-worker
for deployment in nav-api nav-auth nav-calc-worker; do
  ${KUBECTL} -n "${NAMESPACE}" rollout status \
    "deployment/${deployment}" --timeout=300s
done

echo
echo "Observability is ready."
echo "Grafana:    http://nav.local/grafana/"
echo "Prometheus: http://nav.local/prometheus/"
echo "Grafana user: admin"
if [[ "${GRAFANA_ADMIN_PASSWORD}" == "admin" ]]; then
  echo "Grafana password: admin (local default)"
else
  echo "Grafana password is stored in secret observability-secrets."
fi
echo "For a shared environment, set GRAFANA_ADMIN_PASSWORD before the first deploy."
