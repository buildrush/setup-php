#!/usr/bin/env bash
set -euo pipefail

# Build a PHP core bundle from source on Linux.
# Required env vars: PHP_VERSION
# Optional env vars: ARCH (default: x86_64), OUTPUT_DIR (default: /tmp/out)

PHP_VERSION="${PHP_VERSION:?PHP_VERSION is required}"
ARCH="${ARCH:-x86_64}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/out}"
WORKSPACE="${WORKSPACE:-$(pwd)}"

echo "Building PHP ${PHP_VERSION} for linux/${ARCH}"

# Install build dependencies
export DEBIAN_FRONTEND=noninteractive
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO apt-get update -qq
$SUDO apt-get install -y -qq \
  autoconf bison re2c pkg-config build-essential \
  libicu-dev libcurl4-openssl-dev libzip-dev libsqlite3-dev \
  libpq-dev libonig-dev libreadline-dev libsodium-dev \
  libfreetype6-dev libjpeg-dev libwebp-dev libxml2-dev \
  zlib1g-dev libssl-dev gnupg2 xz-utils curl \
  > /dev/null 2>&1

# Download PHP source
PHP_URL="https://www.php.net/distributions/php-${PHP_VERSION}.tar.xz"
echo "Downloading ${PHP_URL}"
curl -sSfL -o /tmp/php.tar.xz "$PHP_URL"

# Extract source
mkdir -p /tmp/php-src
tar -xf /tmp/php.tar.xz -C /tmp/php-src --strip-components=1

# Configure
cd /tmp/php-src
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
  --disable-cgi \
  --enable-opcache \
  > /dev/null 2>&1

# Build
echo "Compiling PHP ${PHP_VERSION}..."
make -j"$(nproc)" > /dev/null 2>&1

# Install to output directory
mkdir -p "$OUTPUT_DIR"
make install INSTALL_ROOT="$OUTPUT_DIR" > /dev/null 2>&1

# Strip binaries
find "${OUTPUT_DIR}/usr/local/bin" -type f -exec strip {} \; 2>/dev/null || true

# Create conf.d directory
mkdir -p "${OUTPUT_DIR}/usr/local/etc/php/conf.d"

# Verify
"${OUTPUT_DIR}/usr/local/bin/php" -v
echo "PHP ${PHP_VERSION} built successfully"

# Pack the bundle
"${WORKSPACE}/builders/common/pack-bundle.sh" "$OUTPUT_DIR" /tmp/bundle.tar.zst
