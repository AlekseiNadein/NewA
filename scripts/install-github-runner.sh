#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${1:?usage: install-github-runner.sh OWNER/REPOSITORY [RUNNER_NAME]}"
RUNNER_NAME="${2:-newa-staging-$(hostname)}"
INSTALL_DIR="${RUNNER_INSTALL_DIR:-/opt/actions-runner}"

command -v gh >/dev/null
command -v curl >/dev/null

if [[ -e "${INSTALL_DIR}/.runner" ]]; then
  echo "Runner is already configured in ${INSTALL_DIR}" >&2
  exit 1
fi

mkdir -p "${INSTALL_DIR}"
cd "${INSTALL_DIR}"

version="$(gh api repos/actions/runner/releases/latest --jq .tag_name)"
version="${version#v}"
archive="actions-runner-linux-x64-${version}.tar.gz"
url="https://github.com/actions/runner/releases/download/v${version}/${archive}"

curl --fail --location --retry 3 --output "${archive}" "${url}"
tar xzf "${archive}"
rm -f "${archive}"

token="$(gh api \
  --method POST \
  "repos/${REPOSITORY}/actions/runners/registration-token" \
  --jq .token)"

./config.sh \
  --unattended \
  --url "https://github.com/${REPOSITORY}" \
  --token "${token}" \
  --name "${RUNNER_NAME}" \
  --labels newa-staging \
  --work _work \
  --replace

echo "Runner configured in ${INSTALL_DIR}."
