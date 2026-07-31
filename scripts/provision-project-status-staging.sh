#!/usr/bin/env bash
set -Eeuo pipefail

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
KUBECTL="${KUBECTL:-kubectl}"
DB_ROLE="${PROJECT_STATUS_DB_ROLE:-project_status_ro}"

random_secret() {
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
}

secret_value() {
  local secret="$1"
  local key="$2"
  local encoded
  encoded="$(
    "${KUBECTL}" -n "${NAMESPACE}" get secret "${secret}" \
      -o "jsonpath={.data.${key}}" 2>/dev/null || true
  )"
  if [[ -n "${encoded}" ]]; then
    printf '%s' "${encoded}" | base64 --decode
  fi
}

if ! [[ "${DB_ROLE}" =~ ^[a-z_][a-z0-9_]*$ ]]; then
  echo "PROJECT_STATUS_DB_ROLE must be a simple PostgreSQL identifier" >&2
  exit 1
fi

"${KUBECTL}" -n "${NAMESPACE}" rollout status statefulset/postgres --timeout=600s

APP_JWT_SECRET="$(secret_value nav-secrets APP_JWT_SECRET)"
if [[ -z "${APP_JWT_SECRET}" ]]; then
  echo "${NAMESPACE}/nav-secrets is missing APP_JWT_SECRET" >&2
  exit 1
fi

DB_PASSWORD="$(
  secret_value project-status-secrets DATABASE_PASSWORD
)"
DB_PASSWORD="${DB_PASSWORD:-$(random_secret)}"

echo "Creating or rotating the restricted ${DB_ROLE} PostgreSQL role"
"${KUBECTL}" -n "${NAMESPACE}" exec -i statefulset/postgres -- \
  psql -v ON_ERROR_STOP=1 -U nav -d nav <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${DB_ROLE}') THEN
    CREATE ROLE ${DB_ROLE} LOGIN PASSWORD '${DB_PASSWORD}';
  ELSE
    ALTER ROLE ${DB_ROLE} WITH LOGIN PASSWORD '${DB_PASSWORD}';
  END IF;
END
\$\$;

ALTER ROLE ${DB_ROLE}
  NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
GRANT CONNECT ON DATABASE nav TO ${DB_ROLE};
GRANT USAGE ON SCHEMA public TO ${DB_ROLE};
REVOKE CREATE ON SCHEMA public FROM ${DB_ROLE};
GRANT SELECT ON TABLE
  app_constructions,
  app_construction_objects,
  app_estimates,
  app_estimate_lines,
  estimate_calc_state,
  estimate_calc_lines,
  estimate_calc_line_resources,
  estimate_calc_resources
TO ${DB_ROLE};
SQL

"${KUBECTL}" -n "${NAMESPACE}" create secret generic project-status-secrets \
  --from-literal=DATABASE_PASSWORD="${DB_PASSWORD}" \
  --from-literal=APP_DATABASE_URL="postgres://${DB_ROLE}:${DB_PASSWORD}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_JWT_SECRET="${APP_JWT_SECRET}" \
  --dry-run=client -o yaml |
  "${KUBECTL}" apply -f -

echo "Provisioned ${NAMESPACE}/project-status-secrets with read-only DB access."
