#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KUBECTL="${KUBECTL:-kubectl}"
NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"

random_secret() {
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
}

secret_value() {
  local secret="$1"
  local key="$2"
  local encoded
  encoded="$(${KUBECTL} -n "${NAMESPACE}" get secret "${secret}" \
    -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
  if [[ -n "${encoded}" ]]; then
    printf '%s' "${encoded}" | base64 --decode
  fi
}

echo "[1/5] Applying namespace and RBAC"
${KUBECTL} apply -f "${ROOT_DIR}/deploy/environments/staging/namespace.yaml"
${KUBECTL} apply -f "${ROOT_DIR}/deploy/environments/staging/rbac.yaml"

POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(secret_value nav-secrets POSTGRES_PASSWORD)}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_secret)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-$(secret_value nav-secrets RABBITMQ_DEFAULT_USER)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-nav}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(secret_value nav-secrets RABBITMQ_DEFAULT_PASS)}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(random_secret)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(secret_value nav-secrets APP_JWT_SECRET)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(random_secret)$(random_secret)}"

echo "[2/5] Creating runtime secrets"
${KUBECTL} -n "${NAMESPACE}" create secret generic nav-secrets \
  --from-literal=POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
  --from-literal=RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER}" \
  --from-literal=RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS}" \
  --from-literal=APP_JWT_SECRET="${APP_JWT_SECRET}" \
  --from-literal=APP_DATABASE_URL="postgres://nav:${POSTGRES_PASSWORD}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_AUTH_DATABASE_URL="postgres://nav:${POSTGRES_PASSWORD}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_GSN_DATABASE_URL="postgres://nav:${POSTGRES_PASSWORD}@postgres:5432/nav?sslmode=disable" \
  --from-literal=APP_RABBITMQ_URL="amqp://${RABBITMQ_DEFAULT_USER}:${RABBITMQ_DEFAULT_PASS}@rabbitmq:5672/" \
  --dry-run=client -o yaml | ${KUBECTL} apply -f -

echo "[3/5] Applying lean staging manifests (without observability)"
STAGING_NAMESPACE="${NAMESPACE}" IMAGE_REPOSITORY=nav-saas IMAGE_TAG=dev \
  bash "${ROOT_DIR}/scripts/deploy-staging.sh"

# Local bootstrap uses containerd import later via CI; keep Never until GHCR secret exists.
for deploy in nav-api nav-auth nav-calc-worker; do
  ${KUBECTL} -n "${NAMESPACE}" patch deployment "${deploy}" --type json \
    -p='[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"IfNotPresent"}]' >/dev/null || true
done

echo "[4/5] Waiting for dependencies"
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/postgres --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/rabbitmq --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/redis --timeout=600s

echo "Application Deployments may stay Pending until the first GHCR image is published by CI."

echo "[5/5] Mapping host newa-staging.local"
if ! grep -q 'newa-staging.local' /etc/hosts 2>/dev/null; then
  NODE_IP="$(${KUBECTL} get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
  if [[ -n "${NODE_IP}" ]]; then
    echo "${NODE_IP} newa-staging.local" | tee -a /etc/hosts >/dev/null || \
      echo "Add '${NODE_IP} newa-staging.local' to /etc/hosts as root"
  fi
fi

echo
echo "Staging bootstrap complete."
echo "URL: http://newa-staging.local"
echo "Initial login: admin@example.com / admin123"
