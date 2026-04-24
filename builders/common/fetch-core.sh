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
# Two code paths:
#   - Production/GHCR (default): the registry was seeded with `oras push`,
#     which sets `org.opencontainers.image.title` annotations on each
#     layer. `oras pull` uses those annotations as output filenames and
#     writes bundle.tar.zst into the cwd. The artifact stores files with
#     absolute paths so --allow-path-traversal lets oras write to /tmp.
#   - Sidecar (PHPUP_REGISTRY_PLAIN_HTTP=1): the sidecar was seeded by
#     phpup's SeedCore via go-containerregistry's remote.Write, which
#     does NOT set layer-level title annotations. `oras pull` would
#     skip the unnamed layers. Use `oras manifest fetch` + `oras blob
#     fetch` to bypass the title-annotation requirement — the bundle
#     is always layer 0 (layoutStore.Push / buildTwoLayerImage invariant).
BUNDLE_FILE="/tmp/bundle.tar.zst"
mkdir -p "$CORE_DIR"
if [ "${PHPUP_REGISTRY_PLAIN_HTTP:-0}" = "1" ]; then
  MANIFEST_JSON=$(oras manifest fetch --plain-http "${REGISTRY}/php-core:${TAG}")
  BUNDLE_DIGEST=$(echo "$MANIFEST_JSON" | jq -r '.layers[0].digest')
  if [ -z "$BUNDLE_DIGEST" ] || [ "$BUNDLE_DIGEST" = "null" ]; then
    echo "Error: manifest at ${REGISTRY}/php-core:${TAG} has no layers"
    exit 1
  fi
  oras blob fetch --plain-http --output "$BUNDLE_FILE" \
    "${REGISTRY}/php-core@${BUNDLE_DIGEST}"
else
  oras pull --allow-path-traversal "${REGISTRY}/php-core:${TAG}"
fi

# Extract the tarball
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
