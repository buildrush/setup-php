#!/usr/bin/env bash
set -euo pipefail

# Build a PHP extension bundle on Linux.
# Required env vars: EXT_NAME, EXT_VERSION, PHP_ABI
# Optional env vars: OUTPUT_DIR (default: /tmp/ext-out)

EXT_NAME="${EXT_NAME:?EXT_NAME is required}"
EXT_VERSION="${EXT_VERSION:?EXT_VERSION is required}"
PHP_ABI="${PHP_ABI:?PHP_ABI is required}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/ext-out}"
WORKSPACE="${WORKSPACE:-$(pwd)}"

echo "Building extension ${EXT_NAME} ${EXT_VERSION} for PHP ${PHP_ABI}"

# Install build dependencies
export DEBIAN_FRONTEND=noninteractive
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
echo "::group::Installing build dependencies"
$SUDO apt-get update -qq
$SUDO apt-get install -y -qq build-essential autoconf pkg-config curl
echo "::endgroup::"

# Per-extension build_deps from catalog (populated by the workflow via yq).
# Existing extensions with no build_deps block yield empty BUILD_DEPS → no-op.
if [ -n "${BUILD_DEPS:-}" ]; then
  echo "::group::Installing per-extension build_deps: ${BUILD_DEPS}"
  # shellcheck disable=SC2086
  $SUDO apt-get install -y -qq --no-install-recommends ${BUILD_DEPS}
  echo "::endgroup::"
fi

# Fetch PHP core bundle (provides phpize, php-config, headers)
PHP_TS=$(echo "$PHP_ABI" | rev | cut -d- -f1 | rev)
PHP_VER=$(echo "$PHP_ABI" | rev | cut -d- -f2- | rev)
echo "Fetching PHP core for ABI ${PHP_ABI}"
"${WORKSPACE}/builders/common/fetch-core.sh" "$PHP_ABI" linux x86_64

# Symlink so that phpize/php-config resolve their compiled prefix correctly.
# PHP was built with --prefix=/usr/local but extracted to /opt/buildrush/core/usr/local.
$SUDO rm -rf /usr/local/include/php /usr/local/lib/php /usr/local/bin/php* /usr/local/bin/phpize /usr/local/bin/php-config
$SUDO ln -sf /opt/buildrush/core/usr/local/bin/php /usr/local/bin/php
$SUDO ln -sf /opt/buildrush/core/usr/local/bin/phpize /usr/local/bin/phpize
$SUDO ln -sf /opt/buildrush/core/usr/local/bin/php-config /usr/local/bin/php-config
$SUDO ln -sf /opt/buildrush/core/usr/local/include/php /usr/local/include/php
$SUDO ln -sf /opt/buildrush/core/usr/local/lib/php /usr/local/lib/php
export PATH="/usr/local/bin:$PATH"

# Download extension source from PECL
PECL_URL="https://pecl.php.net/get/${EXT_NAME}-${EXT_VERSION}.tgz"
echo "Downloading ${PECL_URL}"
curl -sSfL -o /tmp/ext.tgz "$PECL_URL"

# Extract
mkdir -p /tmp/ext-src
tar -xf /tmp/ext.tgz -C /tmp/ext-src --strip-components=1

# Build
echo "::group::Building ${EXT_NAME} ${EXT_VERSION}"
cd /tmp/ext-src
phpize
./configure
make -j"$(nproc)"
echo "::endgroup::"

# Install to output directory
echo "::group::Installing to ${OUTPUT_DIR}"
mkdir -p "$OUTPUT_DIR"
make install INSTALL_ROOT="$OUTPUT_DIR"
echo "::endgroup::"

# Find the .so file
SO_FILE=$(find "$OUTPUT_DIR" -name "${EXT_NAME}.so" | head -1)
if [ -z "$SO_FILE" ]; then
  echo "Error: ${EXT_NAME}.so not found after build"
  exit 1
fi

echo "Extension ${EXT_NAME}.so built at ${SO_FILE}"

# Pack the bundle
"${WORKSPACE}/builders/common/pack-bundle.sh" php-ext "$OUTPUT_DIR" /tmp/bundle.tar.zst
