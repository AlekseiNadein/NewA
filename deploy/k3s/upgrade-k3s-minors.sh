#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root." >&2
  exit 1
fi

INSTALL_SCRIPT="${INSTALL_SCRIPT:-/tmp/get-k3s.sh}"
K3S_EXEC="${K3S_EXEC:-server --write-kubeconfig-mode 644}"

versions=(
  "v1.25.16+k3s4"
  "v1.26.15+k3s1"
  "v1.27.16+k3s1"
  "v1.28.15+k3s1"
  "v1.29.15+k3s1"
  "v1.30.14+k3s2"
  "v1.31.14+k3s1"
  "v1.32.13+k3s1"
  "v1.33.13+k3s1"
  "v1.34.9+k3s1"
  "v1.35.6+k3s1"
  "v1.36.2+k3s1"
)

if [[ ! -s "${INSTALL_SCRIPT}" ]]; then
  curl -fsSL https://get.k3s.io -o "${INSTALL_SCRIPT}"
fi

node_name="$(k3s kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
current_minor="$(k3s --version | awk 'NR == 1 { split($3, p, "."); print p[2] }')"

for version in "${versions[@]}"; do
  target_minor="$(awk -F. '{ print $2 }' <<<"${version}")"
  if (( target_minor <= current_minor )); then
    continue
  fi

  echo "UPGRADE_START ${version}"

  # Old kubelets on WSL cannot parse the Docker Desktop mount whose source
  # contains an unescaped space ("C:\Program Files\...").
  if mountpoint -q /Docker/host; then
    umount /Docker/host
  fi

  env \
    INSTALL_K3S_VERSION="${version}" \
    INSTALL_K3S_EXEC="${K3S_EXEC}" \
    sh "${INSTALL_SCRIPT}"

  systemctl is-active --quiet k3s
  k3s kubectl wait "node/${node_name}" --for=condition=Ready --timeout=180s
  k3s kubectl -n kube-system rollout status deployment/traefik --timeout=300s

  for attempt in $(seq 1 60); do
    if curl -fsS --connect-timeout 3 --max-time 10 -H 'Host: nav.local' \
      http://127.0.0.1/api/healthz >/dev/null; then
      break
    fi
    if [[ "${attempt}" == "60" ]]; then
      echo "NAV ingress health check failed after ${version}." >&2
      exit 1
    fi
    sleep 2
  done

  actual="$(k3s --version | awk 'NR == 1 { print $3 }')"
  if [[ "${actual}" != "${version}" ]]; then
    echo "Expected ${version}, got ${actual}." >&2
    exit 1
  fi

  current_minor="${target_minor}"
  echo "UPGRADE_OK ${version}"
done

echo "All requested k3s minor upgrades completed."
