#!/usr/bin/env bash
set -euo pipefail

# Fetch a PHP core bundle from GHCR.
# Usage: fetch-core.sh <php-abi> <os> <arch>

PHP_ABI="${1:?Usage: fetch-core.sh <php-abi> <os> <arch>}"
OS="${2:?}"
ARCH="${3:?}"

REGISTRY="${REGISTRY:-ghcr.io/buildrush}"
CORE_DIR="${CORE_DIR:-/opt/buildrush/core}"

# Split PHP_ABI (e.g. "8.4-nts") into version and thread safety
PHP_VER=$(echo "$PHP_ABI" | rev | cut -d- -f2- | rev)
PHP_TS=$(echo "$PHP_ABI" | rev | cut -d- -f1 | rev)
TAG="${PHP_VER}-${OS}-${ARCH}-${PHP_TS}"

echo "Fetching PHP core: ${TAG} from ${REGISTRY}"

# Login to GHCR
if [ -n "${GHCR_TOKEN:-}" ]; then
  echo "$GHCR_TOKEN" | oras login ghcr.io -u token --password-stdin
fi

# Pull the core bundle
# The OCI artifact stores files with absolute paths (/tmp/bundle.tar.zst),
# so oras writes them there regardless of -o flag.
mkdir -p "$CORE_DIR"
oras pull --allow-path-traversal "${REGISTRY}/php-core:${TAG}"

# Extract the tarball
BUNDLE_FILE="/tmp/bundle.tar.zst"
if [ ! -f "$BUNDLE_FILE" ]; then
  echo "Error: ${BUNDLE_FILE} not found after pull"
  exit 1
fi

tar --zstd -xf "$BUNDLE_FILE" -C "$CORE_DIR"

# Verify key files exist
for f in bin/php bin/phpize bin/php-config; do
  if [ ! -f "${CORE_DIR}/usr/local/${f}" ]; then
    echo "Error: ${f} not found in core bundle"
    exit 1
  fi
done

export PHP_HOME="${CORE_DIR}/usr/local"
echo "PHP core extracted to $PHP_HOME"
