#!/usr/bin/env bash
set -Eeuo pipefail

NAMESPACE="${NAMESPACE:-nav}"
KUBECTL="${KUBECTL:-k3s kubectl}"
INGRESS_URL="${INGRESS_URL:-http://127.0.0.1}"
INGRESS_HOST="${INGRESS_HOST:-nav.local}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local content="$1"
  local expected="$2"
  local description="$3"
  if [[ "${content}" != *"${expected}"* ]]; then
    fail "${description}: expected '${expected}'"
  fi
  echo "OK: ${description}"
}

for deployment in prometheus grafana loki tempo otel-collector alloy; do
  available="$(${KUBECTL} -n "${NAMESPACE}" get "deployment/${deployment}" \
    -o jsonpath='{.status.availableReplicas}')"
  [[ "${available}" == "1" ]] || fail "deployment/${deployment} is not available"
done
echo "OK: all observability deployments are available"

curl -fsS -H "Host: ${INGRESS_HOST}" \
  "${INGRESS_URL}/api/healthz" >/dev/null
sleep 6

prometheus="$(${KUBECTL} get --raw \
  "/api/v1/namespaces/${NAMESPACE}/services/http:prometheus:9090/proxy/prometheus/api/v1/query?query=up%7Bjob%3D~%22nav-.%2B%22%7D")"
for service in nav-api nav-auth nav-calc-worker; do
  assert_contains "${prometheus}" "\"job\":\"${service}\"" \
    "Prometheus scrapes ${service}"
done

loki="$(${KUBECTL} get --raw \
  "/api/v1/namespaces/${NAMESPACE}/services/http:loki:3100/proxy/loki/api/v1/query_range?query=%7Bjob%3D%22nav%22%2Cservice%3D%22nav-api%22%7D%20%7C%20json%20%7C%20trace_id%21%3D%22%22&limit=5")"
assert_contains "${loki}" 'trace_id' "Loki contains correlated NAV logs"

tempo="$(${KUBECTL} get --raw \
  "/api/v1/namespaces/${NAMESPACE}/services/http:tempo:3200/proxy/api/search?limit=5")"
assert_contains "${tempo}" '"rootServiceName":"nav-api"' \
  "Tempo contains nav-api traces"

grafana_password="$(${KUBECTL} -n "${NAMESPACE}" get secret \
  observability-secrets -o jsonpath='{.data.GRAFANA_ADMIN_PASSWORD}' | \
  base64 --decode)"

grafana_health="$(curl -fsS -u "admin:${grafana_password}" \
  -H "Host: ${INGRESS_HOST}" "${INGRESS_URL}/grafana/api/health")"
assert_contains "${grafana_health}" '"database"' "Grafana ingress health response"
assert_contains "${grafana_health}" '"ok"' "Grafana database health"

dashboard="$(curl -fsS -u "admin:${grafana_password}" \
  -H "Host: ${INGRESS_HOST}" \
  "${INGRESS_URL}/grafana/api/search?query=NAV%20Overview")"
assert_contains "${dashboard}" '"uid":"nav-overview"' \
  "Grafana provisioned NAV Overview"

k6_dashboard="$(curl -fsS -u "admin:${grafana_password}" \
  -H "Host: ${INGRESS_HOST}" \
  "${INGRESS_URL}/grafana/api/dashboards/uid/nav-k6-load")"
assert_contains "${k6_dashboard}" '"uid":"nav-k6-load"' \
  "Grafana provisioned k6 Load Test"

prometheus_flags="$(${KUBECTL} -n "${NAMESPACE}" get deployment/prometheus \
  -o jsonpath='{.spec.template.spec.containers[0].args}')"
assert_contains "${prometheus_flags}" '--web.enable-remote-write-receiver' \
  "Prometheus accepts k6 remote-write metrics"

for datasource in prometheus loki; do
  health="$(curl -fsS -u "admin:${grafana_password}" \
    -H "Host: ${INGRESS_HOST}" \
    "${INGRESS_URL}/grafana/api/datasources/uid/${datasource}/health")"
  assert_contains "${health}" '"status"' \
    "Grafana datasource ${datasource} health response"
  assert_contains "${health}" '"OK"' \
    "Grafana datasource ${datasource}"
done

tempo_datasource="$(curl -fsS -u "admin:${grafana_password}" \
  -H "Host: ${INGRESS_HOST}" \
  "${INGRESS_URL}/grafana/api/datasources/uid/tempo")"
assert_contains "${tempo_datasource}" '"uid":"tempo"' \
  "Grafana provisioned Tempo datasource"

rules="$(${KUBECTL} get --raw \
  "/api/v1/namespaces/${NAMESPACE}/services/http:prometheus:9090/proxy/prometheus/api/v1/rules?type=alert")"
assert_contains "${rules}" '"name":"NavServiceDown"' \
  "Prometheus loaded NAV alert rules"

echo "Observability smoke test passed."
