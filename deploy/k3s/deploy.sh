#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${IMAGE:-nav-saas:dev}"
NAMESPACE="${NAMESPACE:-nav}"
KUBECTL="${KUBECTL:-k3s kubectl}"
K3S="${K3S:-k3s}"

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

import_image() {
  local archive="$1"

  if ${K3S} ctr images ls >/dev/null 2>&1; then
    ${K3S} ctr images import "${archive}"
    return
  fi

  if command -v docker >/dev/null 2>&1; then
    echo "Direct containerd access is unavailable; importing through the locally built image."
    docker run --rm \
      --user 0:0 \
      --entrypoint /host/k3s \
      --volume /usr/local/bin/k3s:/host/k3s:ro \
      --volume /run/k3s/containerd/containerd.sock:/run/k3s/containerd/containerd.sock \
      --volume "${archive}:/image.tar:ro" \
      "${IMAGE}" \
      ctr images import /image.tar
    return
  fi

  echo "Cannot access k3s containerd. Run: sudo k3s ctr images import ${archive}" >&2
  exit 1
}

echo "[1/6] Creating namespace"
${KUBECTL} apply -f "${ROOT_DIR}/deploy/k3s/namespace.yaml"

POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(secret_value nav-secrets POSTGRES_PASSWORD)}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_secret)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-$(secret_value nav-secrets RABBITMQ_DEFAULT_USER)}"
RABBITMQ_DEFAULT_USER="${RABBITMQ_DEFAULT_USER:-nav}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(secret_value nav-secrets RABBITMQ_DEFAULT_PASS)}"
RABBITMQ_DEFAULT_PASS="${RABBITMQ_DEFAULT_PASS:-$(random_secret)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(secret_value nav-secrets APP_JWT_SECRET)}"
APP_JWT_SECRET="${APP_JWT_SECRET:-$(random_secret)$(random_secret)}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-$(secret_value observability-secrets GRAFANA_ADMIN_PASSWORD)}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-admin}"

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
import_image "${archive}"

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

${KUBECTL} -n "${NAMESPACE}" create secret generic observability-secrets \
  --from-literal=GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD}" \
  --dry-run=client -o yaml | ${KUBECTL} apply -f -

echo "[5/6] Applying manifests"
deployments_existed=0
if ${KUBECTL} -n "${NAMESPACE}" get deployment nav-api >/dev/null 2>&1; then
  deployments_existed=1
fi
observability_existed=0
if ${KUBECTL} -n "${NAMESPACE}" get deployment prometheus >/dev/null 2>&1; then
  observability_existed=1
fi
${KUBECTL} apply -k "${ROOT_DIR}/deploy/k3s"
if [[ "${observability_existed}" == "1" ]]; then
  ${KUBECTL} -n "${NAMESPACE}" rollout restart \
    deployment/prometheus \
    deployment/loki \
    deployment/tempo \
    deployment/grafana
fi

echo "[6/6] Waiting for the application"
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/postgres --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/rabbitmq --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status statefulset/redis --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/prometheus --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/loki --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/tempo --timeout=600s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/grafana --timeout=600s

${KUBECTL} -n "${NAMESPACE}" rollout restart \
  deployment/otel-collector \
  deployment/alloy
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/otel-collector --timeout=300s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/alloy --timeout=300s

if [[ "${deployments_existed}" == "1" ]]; then
  ${KUBECTL} -n "${NAMESPACE}" rollout restart \
    deployment/nav-api \
    deployment/nav-auth \
    deployment/nav-calc-worker
fi
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-auth --timeout=300s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-calc-worker --timeout=300s
${KUBECTL} -n "${NAMESPACE}" rollout status deployment/nav-api --timeout=300s

echo
echo "Deployment is ready."
echo "Add '<K3S_NODE_IP> nav.local' to the Windows hosts file and open http://nav.local"
echo "Initial login: admin@example.com / admin123 (change it immediately)."
