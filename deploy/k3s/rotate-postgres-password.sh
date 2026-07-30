#!/usr/bin/env bash
set -Eeuo pipefail

NAMESPACE="${NAMESPACE:-nav}"
KUBECTL="${KUBECTL:-k3s kubectl}"

secret_value() {
  local key="$1"
  ${KUBECTL} -n "${NAMESPACE}" get secret nav-secrets \
    -o "jsonpath={.data.${key}}" | base64 --decode
}

new_password="${POSTGRES_PASSWORD:-$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')}"
rabbit_user="$(secret_value RABBITMQ_DEFAULT_USER)"
rabbit_password="$(secret_value RABBITMQ_DEFAULT_PASS)"
jwt_secret="$(secret_value APP_JWT_SECRET)"

${KUBECTL} -n "${NAMESPACE}" exec postgres-0 -- \
  psql --set=ON_ERROR_STOP=1 --username=nav --dbname=nav \
  --command="ALTER ROLE nav PASSWORD '${new_password}';"

${KUBECTL} -n "${NAMESPACE}" create secret generic nav-secrets \
  --from-literal=POSTGRES_PASSWORD="${new_password}" \
  --from-literal=RABBITMQ_DEFAULT_USER="${rabbit_user}" \
  --from-literal=RABBITMQ_DEFAULT_PASS="${rabbit_password}" \
  --from-literal=APP_JWT_SECRET="${jwt_secret}" \
  --from-literal=APP_DATABASE_URL="postgres://nav:${new_password}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_AUTH_DATABASE_URL="postgres://nav:${new_password}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_GSN_DATABASE_URL="postgres://nav:${new_password}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_RABBITMQ_URL="amqp://${rabbit_user}:${rabbit_password}@rabbitmq:5672/" \
  --dry-run=client -o yaml | ${KUBECTL} apply -f -

${KUBECTL} -n "${NAMESPACE}" rollout restart \
  deployment/nav-api deployment/nav-auth deployment/nav-calc-worker
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-auth --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-calc-worker --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-api --timeout=180s

echo "PostgreSQL password rotated and application deployments restarted."
