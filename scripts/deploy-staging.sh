#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
KUBECTL="${KUBECTL:-kubectl}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-nav-saas}"
IMAGE_DIGEST="${IMAGE_DIGEST:-}"
IMAGE_REF="${IMAGE_REPOSITORY}:dev"

if [[ -n "${IMAGE_DIGEST}" ]]; then
  IMAGE_REF="${IMAGE_REPOSITORY}@${IMAGE_DIGEST}"
elif [[ -n "${IMAGE_TAG:-}" ]]; then
  IMAGE_REF="${IMAGE_REPOSITORY}:${IMAGE_TAG}"
fi

apply_namespaced() {
  local file="$1"
  ${KUBECTL} apply -n "${NAMESPACE}" -f "${file}"
}

echo "Deploying lean staging stack to ${NAMESPACE} with image ${IMAGE_REF}"

# RBAC for newa-ci-deploy is applied only by bootstrap-staging.sh (admin).
apply_namespaced "${ROOT_DIR}/deploy/k3s/storage.yaml"
apply_namespaced "${ROOT_DIR}/deploy/k3s/postgres.yaml"
apply_namespaced "${ROOT_DIR}/deploy/k3s/rabbitmq.yaml"
apply_namespaced "${ROOT_DIR}/deploy/k3s/redis.yaml"

# Render app + ingress with digest-pinned image and staging pull settings.
${KUBECTL} create --dry-run=client -o yaml -n "${NAMESPACE}" -f "${ROOT_DIR}/deploy/k3s/app.yaml" |
  sed "s|image: nav-saas:dev|image: ${IMAGE_REF}|g" |
  sed "s|imagePullPolicy: Never|imagePullPolicy: Always|g" |
  ${KUBECTL} apply -f -

${KUBECTL} create --dry-run=client -o yaml -n "${NAMESPACE}" -f "${ROOT_DIR}/deploy/k3s/ingress.yaml" |
  sed "s|host: nav.local|host: newa-staging.local|g" |
  ${KUBECTL} apply -f -

# Ensure imagePullSecrets on app pods for the staging registry.
for deploy in nav-api nav-auth nav-calc-worker; do
  ${KUBECTL} -n "${NAMESPACE}" patch deployment "${deploy}" --type strategic -p '{"spec":{"strategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":0,"maxSurge":1}},"revisionHistoryLimit":10,"template":{"spec":{"imagePullSecrets":[{"name":"newa-registry-pull"}]}}}}' >/dev/null
done

echo "Staging manifests applied."
