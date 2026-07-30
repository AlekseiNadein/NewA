#!/usr/bin/env sh
set -eu

NAMESPACE="${STAGING_NAMESPACE:-newa-staging}"

if kubectl auth can-i create namespaces | grep -Eq '^yes$'; then
  echo "RBAC is too broad: deploy identity can create namespaces" >&2
  exit 1
fi

if kubectl auth can-i create clusterrolebindings | grep -Eq '^yes$'; then
  echo "RBAC is too broad: deploy identity can create clusterrolebindings" >&2
  exit 1
fi

if kubectl auth can-i create configmaps -n kube-system | grep -Eq '^yes$'; then
  echo "RBAC is too broad: deploy identity can write to kube-system" >&2
  exit 1
fi

if ! kubectl auth can-i get pods -n "${NAMESPACE}" | grep -Eq '^yes$'; then
  echo "RBAC is too narrow: deploy identity cannot read pods in ${NAMESPACE}" >&2
  exit 1
fi

echo "RBAC negative test passed."
