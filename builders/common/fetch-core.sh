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
mkdir -p "$CORE_DIR"
oras pull --allow-path-traversal "${REGISTRY}/php-core:${TAG}" -o /tmp/core-bundle/

# Extract the tarball
BUNDLE_FILE=$(find /tmp/core-bundle/ -name '*.tar.zst' -o -name '*.tar.zstd' | head -1)
if [ -z "$BUNDLE_FILE" ]; then
  echo "Error: no tarball found in pulled bundle"
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
