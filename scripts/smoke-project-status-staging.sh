#!/usr/bin/env bash
set -Eeuo pipefail

BASE_URL="${STAGING_BASE_URL:?STAGING_BASE_URL is required}"
BASE_URL="${BASE_URL%/}"

command -v curl >/dev/null 2>&1
command -v jq >/dev/null 2>&1

for path in /projectStatusDesktop/ /projectStatusMobile/; do
  curl --fail-with-body --silent --show-error \
    "${BASE_URL}${path}" >/dev/null
done

unauthorized_status="$(
  curl --silent --output /dev/null --write-out '%{http_code}' \
    --header 'Content-Type: application/json' \
    --data '{"query":"{ constructions(first: 1) { totalCount } }"}' \
    "${BASE_URL}/api/project-status/graphql"
)"
if [[ "${unauthorized_status}" != "401" ]]; then
  echo "Expected unauthenticated GraphQL status 401, got ${unauthorized_status}" >&2
  exit 1
fi

if [[ -z "${SMOKE_COMPANY_NAME:-}" || -z "${SMOKE_USER_NAME:-}" || -z "${SMOKE_PASSWORD:-}" ]]; then
  echo "Authenticated GraphQL smoke credentials are required" >&2
  exit 1
fi

cookie_jar="$(mktemp)"
response="$(mktemp)"
cleanup() {
  rm -f "${cookie_jar}" "${response}"
}
trap cleanup EXIT

login_body="$(
  jq -n \
    --arg companyName "${SMOKE_COMPANY_NAME}" \
    --arg name "${SMOKE_USER_NAME}" \
    --arg password "${SMOKE_PASSWORD}" \
    '{companyName: $companyName, name: $name, password: $password}'
)"

curl --fail-with-body --silent --show-error \
  --cookie-jar "${cookie_jar}" \
  --header 'Content-Type: application/json' \
  --data "${login_body}" \
  "${BASE_URL}/api/auth/login" > "${response}"
jq -e '.user' "${response}" >/dev/null

curl --fail-with-body --silent --show-error \
  --cookie "${cookie_jar}" \
  --header 'Content-Type: application/json' \
  --data '{"query":"{ constructions(first: 1) { totalCount edges { node { id } } } }"}' \
  "${BASE_URL}/api/project-status/graphql" > "${response}"

jq -e '
  (((.errors // []) | length) == 0)
  and ((.data.constructions.totalCount | type) == "number")
' "${response}" >/dev/null

echo "ProjectStatus staging smoke test passed."
