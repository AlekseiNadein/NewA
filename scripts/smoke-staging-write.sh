#!/usr/bin/env sh
set -eu

test -n "${STAGING_BASE_URL:-}"
test -n "${SMOKE_COMPANY_NAME:-}"
test -n "${SMOKE_USER_NAME:-}"
test -n "${SMOKE_PASSWORD:-}"

command -v curl >/dev/null 2>&1
command -v jq >/dev/null 2>&1

BASE_URL="${STAGING_BASE_URL%/}"
COOKIE_JAR="$(mktemp)"
RESPONSE="$(mktemp)"
CONSTRUCTION_ID=""
SMOKE_TIMEOUT_SEC="${STAGING_SMOKE_TIMEOUT_SEC:-180}"
SMOKE_INTERVAL_SEC="${STAGING_SMOKE_INTERVAL_SEC:-5}"

cleanup() {
  if [ -n "${CONSTRUCTION_ID}" ]; then
    curl --fail-with-body --silent --show-error --max-time 15 \
      --cookie "${COOKIE_JAR}" \
      --request DELETE \
      "${BASE_URL}/api/constructions/${CONSTRUCTION_ID}" >/dev/null || true
  fi
  rm -f "${COOKIE_JAR}" "${RESPONSE}"
}
trap cleanup EXIT

# Retry transient ingress/API failures (502/503) after rollout.
curl_retry() {
  started="$(date +%s)"
  attempt=0
  while :; do
    attempt=$((attempt + 1))
    if curl --fail-with-body --silent --show-error --max-time 20 "$@"; then
      return 0
    fi
    now="$(date +%s)"
    elapsed=$((now - started))
    if [ "${elapsed}" -ge "${SMOKE_TIMEOUT_SEC}" ]; then
      echo "Smoke HTTP request failed after ${elapsed}s (${attempt} attempts)" >&2
      return 1
    fi
    echo "Smoke HTTP not ready yet (attempt ${attempt}, elapsed ${elapsed}s); retry in ${SMOKE_INTERVAL_SEC}s" >&2
    sleep "${SMOKE_INTERVAL_SEC}"
  done
}

LOGIN_BODY="$(jq -n \
  --arg companyName "${SMOKE_COMPANY_NAME}" \
  --arg name "${SMOKE_USER_NAME}" \
  --arg password "${SMOKE_PASSWORD}" \
  '{companyName: $companyName, name: $name, password: $password}')"

curl_retry \
  --cookie-jar "${COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data "${LOGIN_BODY}" \
  "${BASE_URL}/api/auth/login" > "${RESPONSE}"

jq -e '.user' "${RESPONSE}" >/dev/null

RUN_ID="${CI_PIPELINE_ID:-${GITHUB_RUN_ID:-manual}}"
CREATE_BODY="$(jq -n \
  --arg code "CI-${RUN_ID}" \
  --arg name "CI smoke ${RUN_ID}" \
  '{code: $code, name: $name}')"

curl_retry \
  --cookie "${COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data "${CREATE_BODY}" \
  "${BASE_URL}/api/constructions" > "${RESPONSE}"

CONSTRUCTION_ID="$(jq -er '.id' "${RESPONSE}")"

curl_retry \
  --cookie "${COOKIE_JAR}" \
  "${BASE_URL}/api/constructions" |
  jq -e --arg id "${CONSTRUCTION_ID}" 'any(.[]; .id == $id)' >/dev/null

curl_retry \
  --cookie "${COOKIE_JAR}" \
  --request DELETE \
  "${BASE_URL}/api/constructions/${CONSTRUCTION_ID}" >/dev/null
CONSTRUCTION_ID=""

echo "Staging write smoke test passed."
