#!/usr/bin/env bash
set -euo pipefail

# Run smoke tests for a bundle.
# Usage: run.sh <bundle-name> [<bundle-dir>]

BUNDLE_NAME="${1:?Usage: run.sh <bundle-name> [<bundle-dir>]}"
BUNDLE_DIR="${2:-/tmp/smoke}"
META_PATH="${META_PATH:-/tmp/meta.json}"

echo "Running smoke tests for ${BUNDLE_NAME}"

# Assert the sidecar carries the new schema_version + kind fields. The
# builder's pack-bundle.sh always emits them; missing fields mean a stale
# or hand-crafted bundle.
if [ -f "${META_PATH}" ]; then
  if ! grep -q '"schema_version"' "${META_PATH}"; then
    echo "smoke: meta.json missing schema_version" >&2
    exit 1
  fi
  if ! grep -q '"kind"' "${META_PATH}"; then
    echo "smoke: meta.json missing kind" >&2
    exit 1
  fi
  echo "smoke: sidecar schema_version and kind present"
fi

case "$BUNDLE_NAME" in
  php)
    "${BUNDLE_DIR}/usr/local/bin/php" -v
    "${BUNDLE_DIR}/usr/local/bin/php" -m
    "${BUNDLE_DIR}/usr/local/bin/php" -r "echo PHP_VERSION . PHP_EOL;"
    # Assert upstream ini templates were stashed by the builder.
    if [ ! -s "${BUNDLE_DIR}/usr/local/share/php/ini/php.ini-production" ]; then
      echo "smoke: missing php.ini-production in bundle" >&2
      exit 1
    fi
    if [ ! -s "${BUNDLE_DIR}/usr/local/share/php/ini/php.ini-development" ]; then
      echo "smoke: missing php.ini-development in bundle" >&2
      exit 1
    fi
    echo "smoke: php.ini-production and php.ini-development present"
    echo "PHP core smoke tests passed"
    ;;
  redis)
    # Requires PHP core to be available in PATH
    php -r "assert(extension_loaded('redis'), 'redis not loaded');"
    echo "redis smoke test passed"
    ;;
  *)
    # Generic: try to load the extension
    php -r "assert(extension_loaded('${BUNDLE_NAME}'), '${BUNDLE_NAME} not loaded');"
    echo "${BUNDLE_NAME} smoke test passed"
    ;;
esac
