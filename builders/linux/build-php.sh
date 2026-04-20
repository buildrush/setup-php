#!/usr/bin/env bash
set -euo pipefail

# Build a PHP core bundle from source on Linux.
# Required env vars: PHP_VERSION
# Optional env vars: ARCH (default: x86_64), OUTPUT_DIR (default: /tmp/out)

PHP_VERSION="${PHP_VERSION:?PHP_VERSION is required}"
ARCH="${ARCH:-x86_64}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/out}"
WORKSPACE="${WORKSPACE:-$(pwd)}"
PHP_SRC_DIR="${PHP_SRC_DIR:-/tmp/php-src}"

# Resolve minor version (e.g. "8.4") to latest patch version (e.g. "8.4.20")
if [[ "$PHP_VERSION" =~ ^[0-9]+\.[0-9]+$ ]]; then
  RESOLVED=$(curl -sSf "https://www.php.net/releases/index.php?json&version=${PHP_VERSION}" | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)
  if [ -z "$RESOLVED" ]; then
    echo "Error: could not resolve PHP ${PHP_VERSION} to a patch version"
    exit 1
  fi
  echo "Resolved PHP ${PHP_VERSION} -> ${RESOLVED}"
  PHP_VERSION="$RESOLVED"
fi

echo "Building PHP ${PHP_VERSION} for linux/${ARCH}"

# Install build dependencies
export DEBIAN_FRONTEND=noninteractive
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
echo "::group::Installing build dependencies"
$SUDO apt-get update -qq
$SUDO apt-get install -y -qq \
  autoconf bison re2c pkg-config build-essential \
  libicu-dev libcurl4-openssl-dev libzip-dev libsqlite3-dev \
  libpq-dev libonig-dev libreadline-dev libsodium-dev \
  libfreetype6-dev libjpeg-dev libwebp-dev libxml2-dev \
  zlib1g-dev libssl-dev gnupg2 xz-utils curl \
  libffi-dev gettext
echo "::endgroup::"

# Download PHP source
PHP_URL="https://www.php.net/distributions/php-${PHP_VERSION}.tar.xz"
echo "Downloading ${PHP_URL}"
curl -sSfL -o /tmp/php.tar.xz "$PHP_URL"

# Extract source
mkdir -p "$PHP_SRC_DIR"
tar -xf /tmp/php.tar.xz -C "$PHP_SRC_DIR" --strip-components=1

# Configure
echo "::group::Configuring PHP ${PHP_VERSION}"
cd "$PHP_SRC_DIR"
./configure \
  --prefix=/usr/local \
  --enable-mbstring \
  --with-curl \
  --with-zlib \
  --with-openssl \
  --enable-bcmath \
  --enable-calendar \
  --enable-exif \
  --enable-ftp \
  --enable-intl \
  --with-zip \
  --enable-soap \
  --enable-sockets \
  --with-pdo-mysql \
  --with-pdo-sqlite \
  --with-sqlite3 \
  --with-readline \
  --with-sodium \
  --enable-gd \
  --with-freetype \
  --with-jpeg \
  --with-webp \
  --with-pdo-pgsql \
  --with-pgsql \
  --with-ffi \
  --with-gettext \
  --enable-pcntl \
  --enable-posix \
  --enable-shmop \
  --enable-sysvmsg \
  --enable-sysvsem \
  --enable-sysvshm \
  --disable-cgi \
  --enable-opcache
echo "::endgroup::"

# Build
echo "::group::Compiling PHP ${PHP_VERSION} ($(nproc) cores)"
make -j"$(nproc)"
echo "::endgroup::"

# Install to output directory
echo "::group::Installing to ${OUTPUT_DIR}"
mkdir -p "$OUTPUT_DIR"
make install INSTALL_ROOT="$OUTPUT_DIR"
echo "::endgroup::"

# Strip binaries
find "${OUTPUT_DIR}/usr/local/bin" -type f -exec strip {} \; 2>/dev/null || true

# Create conf.d directory
mkdir -p "${OUTPUT_DIR}/usr/local/etc/php/conf.d"

# Ship PHP's upstream production/development ini templates in the bundle.
# Runtime (internal/compose) selects one based on the `ini-file:` input.
echo "::group::Stashing php.ini-production and php.ini-development"
mkdir -p "${OUTPUT_DIR}/usr/local/share/php/ini"
cp "${PHP_SRC_DIR}/php.ini-production" "${OUTPUT_DIR}/usr/local/share/php/ini/php.ini-production"
cp "${PHP_SRC_DIR}/php.ini-development" "${OUTPUT_DIR}/usr/local/share/php/ini/php.ini-development"
echo "::endgroup::"

# Verify
"${OUTPUT_DIR}/usr/local/bin/php" -v
echo "PHP ${PHP_VERSION} built successfully"

# Pack the bundle
"${WORKSPACE}/builders/common/pack-bundle.sh" "$OUTPUT_DIR" /tmp/bundle.tar.zst
