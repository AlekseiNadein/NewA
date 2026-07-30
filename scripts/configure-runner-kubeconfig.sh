#!/usr/bin/env bash
set -Eeuo pipefail

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"
SA_NAME="${STAGING_SA_NAME:-newa-ci-deploy}"
TOKEN_SECRET="${STAGING_TOKEN_SECRET:-newa-ci-deploy-token}"
KUBECONFIG_OUT="${1:-${HOME}/.kube/newa-staging-ci}"
KUBECTL="${KUBECTL:-kubectl}"

mkdir -p "$(dirname "${KUBECONFIG_OUT}")"

SERVER="$("${KUBECTL}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
CA_DATA="$("${KUBECTL}" config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')"

# Prefer CA from k3s if present for reliable in-cluster TLS.
if [[ -f /var/lib/rancher/k3s/server/tls/server-ca.crt ]]; then
  CA_DATA="$(base64 -w0 /var/lib/rancher/k3s/server/tls/server-ca.crt 2>/dev/null || base64 /var/lib/rancher/k3s/server/tls/server-ca.crt | tr -d '\n')"
fi

for _ in $(seq 1 30); do
  TOKEN="$("${KUBECTL}" -n "${NAMESPACE}" get secret "${TOKEN_SECRET}" -o jsonpath='{.data.token}' 2>/dev/null || true)"
  if [[ -n "${TOKEN}" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "${TOKEN}" ]]; then
  echo "Service account token secret ${TOKEN_SECRET} is empty" >&2
  exit 1
fi
TOKEN="$(printf '%s' "${TOKEN}" | base64 --decode)"

umask 077
cat > "${KUBECONFIG_OUT}" <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: newa-staging
    cluster:
      server: ${SERVER}
      certificate-authority-data: ${CA_DATA}
contexts:
  - name: newa-staging-ci
    context:
      cluster: newa-staging
      namespace: ${NAMESPACE}
      user: ${SA_NAME}
current-context: newa-staging-ci
users:
  - name: ${SA_NAME}
    user:
      token: ${TOKEN}
EOF

echo "Wrote namespace-scoped kubeconfig: ${KUBECONFIG_OUT}"
KUBECONFIG="${KUBECONFIG_OUT}" ${KUBECTL} auth can-i get pods -n "${NAMESPACE}"
KUBECONFIG="${KUBECONFIG_OUT}" ${KUBECTL} auth can-i create namespaces || true
