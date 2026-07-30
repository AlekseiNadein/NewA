#!/usr/bin/env sh
set -eu

mkdir -p out

if [ -z "${IMAGE_REPOSITORY:-}" ] || [ -z "${IMAGE_DIGEST:-}" ]; then
  echo "IMAGE_REPOSITORY and IMAGE_DIGEST are required" >&2
  exit 1
fi

cat > out/last-known-good.env <<EOF
LAST_KNOWN_GOOD_IMAGE=${IMAGE_REPOSITORY}@${IMAGE_DIGEST}
LAST_KNOWN_GOOD_COMMIT=${CI_COMMIT_SHA:-}
LAST_KNOWN_GOOD_PIPELINE=${CI_PIPELINE_ID:-}
LAST_KNOWN_GOOD_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "Recorded last known good image: ${IMAGE_REPOSITORY}@${IMAGE_DIGEST}"
