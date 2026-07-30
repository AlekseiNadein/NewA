#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KUBECTL="${KUBECTL:-k3s kubectl}"
PROXY_NAME="${PROXY_NAME:-nav-k3s-proxy}"
WINDOWS_PORT="${WINDOWS_PORT:-8088}"

node_name="$(${KUBECTL} get nodes -o jsonpath='{.items[0].metadata.name}')"
k3s_ip="$(${KUBECTL} get node "${node_name}" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')"

if [[ -z "${k3s_ip}" ]]; then
  echo "Could not determine the k3s node InternalIP." >&2
  exit 1
fi

if docker container inspect "${PROXY_NAME}" >/dev/null 2>&1; then
  docker rm --force "${PROXY_NAME}" >/dev/null
fi

docker run --detach \
  --name "${PROXY_NAME}" \
  --restart unless-stopped \
  --publish "127.0.0.1:${WINDOWS_PORT}:80" \
  --env NAV_K3S_IP="${k3s_ip}" \
  --volume "${ROOT_DIR}/deploy/k3s/windows-proxy.conf.template:/etc/nginx/templates/default.conf.template:ro" \
  nginx:latest >/dev/null

echo "NAV proxy is listening on http://127.0.0.1:${WINDOWS_PORT} (k3s node ${k3s_ip})."
