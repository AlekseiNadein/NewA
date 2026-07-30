#!/usr/bin/env sh
set -eu

mkdir -p out

BASE_SHA="${CI_MERGE_REQUEST_DIFF_BASE_SHA:-${CI_COMMIT_BEFORE_SHA:-}}"
if [ -z "${BASE_SHA}" ] || [ "${BASE_SHA}" = "0000000000000000000000000000000000000000" ]; then
  BASE_SHA="$(git rev-list --max-count=1 HEAD^ 2>/dev/null || true)"
fi
if [ -z "${BASE_SHA}" ]; then
  BASE_SHA="$(git rev-list --max-count=1 HEAD)"
fi

CHANGED_FILES="$(git diff --name-only "${BASE_SHA}" "${CI_COMMIT_SHA:-HEAD}" || true)"

AFFECTS_APP=0
echo "$CHANGED_FILES" | while IFS= read -r file; do
  case "$file" in
    backend/*|web/*|db/*|Dockerfile|go.mod|go.sum)
      AFFECTS_APP=1
      ;;
  esac
done

# POSIX sh uses subshell in pipeline; re-check using grep to persist value.
if echo "$CHANGED_FILES" | grep -Eq '^(backend/|web/|db/|Dockerfile|go\.mod|go\.sum)'; then
  AFFECTS_APP=1
fi

cat > out/affected-components.json <<EOF
{
  "components": [
    {
      "name": "nav-saas",
      "affected": ${AFFECTS_APP}
    }
  ]
}
EOF

cat > out/affected-components.env <<EOF
AFFECTS_NAV_SAAS=${AFFECTS_APP}
EOF

echo "Affected nav-saas: ${AFFECTS_APP}"
