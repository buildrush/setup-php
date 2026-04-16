#!/usr/bin/env bash
set -euo pipefail

# Run smoke tests for a bundle.
# Usage: run.sh <bundle-name> [<bundle-dir>]

BUNDLE_NAME="${1:?Usage: run.sh <bundle-name> [<bundle-dir>]}"
BUNDLE_DIR="${2:-/tmp/smoke}"

echo "Running smoke tests for ${BUNDLE_NAME}"

case "$BUNDLE_NAME" in
  php)
    "${BUNDLE_DIR}/usr/local/bin/php" -v
    "${BUNDLE_DIR}/usr/local/bin/php" -m
    "${BUNDLE_DIR}/usr/local/bin/php" -r "echo PHP_VERSION . PHP_EOL;"
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
