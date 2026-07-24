#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${IMAGE:-nav-saas:dev}"
NAMESPACE="${NAMESPACE:-nav}"
KUBECTL="${KUBECTL:-sudo k3s kubectl}"
K3S="${K3S:-sudo k3s}"

random_secret() {
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
}

secret_value() {
  local key="$1"
  local encoded
  encoded="$(${KUBECTL} -n "${NAMESPACE}" get secret nav-secrets \
    -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
  if [[ -n "${encoded}" ]]; then
    printf '%s' "${encoded}" | base64 --decode
  fi
}

echo "[1/6] Creating namespace"
${KUBECTL} apply -f "${ROOT_DIR}/deploy/k3s/namespace.yaml"

POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(secret_value POSTGRES_PASSWORD)}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_secret)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-$(secret_value RABBITMQ_DEFAULT_USER)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-nav}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(secret_value RABBITMQ_DEFAULT_PASS)}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(random_secret)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(secret_value APP_JWT_SECRET)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(random_secret)$(random_secret)}"

echo "[2/6] Building ${IMAGE}"
if command -v docker >/dev/null 2>&1; then
  docker build -t "${IMAGE}" "${ROOT_DIR}"
  archive="$(mktemp --suffix=.tar)"
  trap 'rm -f "${archive}"' EXIT
  docker save "${IMAGE}" -o "${archive}"
elif command -v podman >/dev/null 2>&1; then
  podman build -t "${IMAGE}" "${ROOT_DIR}"
  archive="$(mktemp --suffix=.tar)"
  trap 'rm -f "${archive}"' EXIT
  podman save --format docker-archive "${IMAGE}" -o "${archive}"
else
  echo "Docker or Podman is required to build the image." >&2
  exit 1
fi

echo "[3/6] Importing the image into k3s containerd"
${K3S} ctr images import "${archive}"

echo "[4/6] Creating runtime secrets"
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

echo "[5/6] Applying manifests"
${KUBECTL} apply -k "${ROOT_DIR}/deploy/k3s"
${KUBECTL} -n "${NAMESPACE}" rollout restart deployment/nav-api deployment/nav-auth deployment/nav-calc-worker

echo "[6/6] Waiting for the application"
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/postgres --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/rabbitmq --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/redis --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-auth --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-calc-worker --timeout=180s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-api --timeout=180s

echo
echo "Deployment is ready."
echo "Add '<K3S_NODE_IP> nav.local' to the Windows hosts file and open http://nav.local"
echo "Initial login: admin@example.com / admin123 (change it immediately)."
