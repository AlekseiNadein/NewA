#!/usr/bin/env bash
# Full PostgreSQL copy: namespace nav -> newa-staging (same k3s cluster).
# Replaces target database contents. Does not sync Redis/RabbitMQ.
set -Eeuo pipefail

SOURCE_NS="${SOURCE_NS:-nav}"
TARGET_NS="${TARGET_NS:-newa-staging}"
SOURCE_POD="${SOURCE_POD:-postgres-0}"
TARGET_POD="${TARGET_POD:-postgres-0}"
DUMP_PATH="${DUMP_PATH:-/tmp/nav-to-staging.dump}"
HOST_DUMP="${HOST_DUMP:-/tmp/nav-to-staging.dump}"

echo "[1/7] Scaling down staging app workloads"
kubectl -n "${TARGET_NS}" scale deployment/nav-api deployment/nav-auth deployment/nav-calc-worker --replicas=0
kubectl -n "${TARGET_NS}" rollout status deployment/nav-api --timeout=120s || true
kubectl -n "${TARGET_NS}" rollout status deployment/nav-auth --timeout=120s || true
kubectl -n "${TARGET_NS}" rollout status deployment/nav-calc-worker --timeout=120s || true

echo "[2/7] Dumping ${SOURCE_NS}/${SOURCE_POD} database nav"
kubectl -n "${SOURCE_NS}" exec "${SOURCE_POD}" -- \
  bash -lc "rm -f '${DUMP_PATH}' && pg_dump --format=custom --no-owner --no-acl --username=nav --dbname=nav --file='${DUMP_PATH}' && ls -lh '${DUMP_PATH}'"

echo "[3/7] Copying dump via host ${HOST_DUMP}"
rm -f "${HOST_DUMP}"
kubectl -n "${SOURCE_NS}" cp "${SOURCE_POD}:${DUMP_PATH}" "${HOST_DUMP}"
ls -lh "${HOST_DUMP}"
kubectl -n "${TARGET_NS}" cp "${HOST_DUMP}" "${TARGET_POD}:${DUMP_PATH}"
kubectl -n "${SOURCE_NS}" exec "${SOURCE_POD}" -- rm -f "${DUMP_PATH}" || true

echo "[4/7] Recreating target database (avoids FK --clean conflicts)"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- \
  psql -U nav -d postgres -v ON_ERROR_STOP=1 -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'nav' AND pid <> pg_backend_pid();"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- \
  psql -U nav -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS nav;"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- \
  psql -U nav -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE nav OWNER nav;"

echo "[5/7] Restoring into ${TARGET_NS}/${TARGET_POD}"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- \
  bash -lc "pg_restore --no-owner --no-acl --username=nav --dbname=nav '${DUMP_PATH}'; rm -f '${DUMP_PATH}'"

echo "[6/7] Scaling staging apps back up"
kubectl -n "${TARGET_NS}" scale deployment/nav-api deployment/nav-auth deployment/nav-calc-worker --replicas=1
kubectl -n "${TARGET_NS}" rollout status deployment/nav-auth --timeout=300s
kubectl -n "${TARGET_NS}" rollout status deployment/nav-calc-worker --timeout=300s
kubectl -n "${TARGET_NS}" rollout status deployment/nav-api --timeout=300s

echo "[7/7] Verification"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- psql -U nav -d nav -c "
SELECT 'auth.users' AS item, count(*)::text AS value FROM auth.users
UNION ALL SELECT 'auth.companies', count(*)::text FROM auth.companies
UNION ALL SELECT 'gsn.records', count(*)::text FROM gsn.records
UNION ALL SELECT 'app_estimates', count(*)::text FROM app_estimates
UNION ALL SELECT 'app_estimate_lines', count(*)::text FROM app_estimate_lines
UNION ALL SELECT 'fgis_cs.set_rows', count(*)::text FROM fgis_cs.set_rows
UNION ALL SELECT 'outbox_events', count(*)::text FROM outbox_events
ORDER BY 1;
"
kubectl -n "${TARGET_NS}" exec "${TARGET_POD}" -- psql -U nav -d nav -c "
SELECT u.name, u.email, c.name AS company
FROM auth.users u
LEFT JOIN auth.companies c ON c.id = u.company_id
ORDER BY u.name;
"

rm -f "${HOST_DUMP}"
echo "Database sync ${SOURCE_NS} -> ${TARGET_NS} completed."
