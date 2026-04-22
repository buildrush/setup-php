#!/usr/bin/env bash
# local-ci.sh — reproduce the compat-harness's bundle-loading tests on the
# host machine via Docker, without waiting 30+ minutes for CI.
#
# What it does:
#   1. Pulls the currently-published php-core + requested extension bundles
#      from GHCR (anonymous read OK for public packages).
#   2. Composes a mock /opt/buildrush tree locally.
#   3. Runs the tree inside both ubuntu:22.04 (jammy) and ubuntu:24.04 (noble)
#      containers to exercise the cross-OS invariant.
#   4. For each (runner_os, fixture), installs the core runtime deps + the
#      fixture's extension runtime_deps (read from catalog via yq), then
#      loads php with the extension set and asserts every requested extension
#      is reported as loaded.
#
# Usage:
#   test/smoke/local-ci.sh                  # default: 8.4 + bare + hard4 fixtures
#   test/smoke/local-ci.sh --php 8.3        # override PHP version
#   test/smoke/local-ci.sh --arch x86_64    # override arch (requires --platform emulation on ARM hosts)
#
# Requires: docker, jq, oras, yq on the host. zstd + tar used inside containers.

set -euo pipefail

PHP_VERSION="8.4"
# Default: exercise BOTH arches. x86_64 runs under docker's QEMU on ARM hosts
# (slower but catches x86-only issues like pgdg picking on GH's x86 runners).
# Override with --arch to scope to one.
ARCHES="aarch64 x86_64"
REGISTRY="ghcr.io/buildrush"
WORKSPACE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="${TMPDIR:-/tmp}/local-ci-$$"
KEEP_WORK=false

# Fixtures to exercise. Each entry is "<name>|<extensions>". An empty extensions
# value means "bare" (core only).
FIXTURES=(
  "bare|"
  "hard4|imagick,mongodb,swoole,grpc"
)

RUNNERS=("ubuntu-22.04" "ubuntu-24.04")

# Core system-level runtime deps that GH Actions runners ship preinstalled but
# minimal Ubuntu base images do not. Scraped from the NEEDED list of a core
# bundle's php binary on 2026-04-22; keep in sync if core configure flags change.
CORE_RUNTIME_DEPS=(
  libreadline8 libssl3 libcurl4 libxml2 libonig5 libpq5 libsqlite3-0
  libsodium23 libzip4 libffi8 libpng16-16 libwebp7 libjpeg8 libfreetype6
  libgomp1 libxext6
)

usage() {
  sed -n '2,/^$/p' "$0" | sed 's/^# \?//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --php) PHP_VERSION="$2"; shift 2 ;;
    --arch) ARCHES="$2"; shift 2 ;;
    --keep) KEEP_WORK=true; shift ;;
    -h|--help) usage ;;
    *) echo "unknown arg: $1" >&2; usage ;;
  esac
done

for cmd in docker jq oras yq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "::error::missing required command: $cmd" >&2
    exit 2
  fi
done

mkdir -p "$WORK"
cleanup() {
  if ! $KEEP_WORK; then rm -rf "$WORK"; fi
}
trap cleanup EXIT

echo "=== local-ci: PHP $PHP_VERSION / arches=${ARCHES} ==="
echo "workspace: $WORKSPACE"
echo "scratch:   $WORK (KEEP=$KEEP_WORK)"

# Pull + extract one bundle into $WORK/bundles/<kind>/<name>/
# Kinds: "core" (whole php-core), "ext" (extension named $2).
pull_bundle() {
  local kind="$1" name="${2:-}"
  local tag dest ref
  case "$kind" in
    core)
      tag="${PHP_VERSION}-linux-${ARCH}-nts"
      ref="${REGISTRY}/php-core:${tag}"
      dest="$WORK/bundles/core"
      ;;
    ext)
      # Look up version from the catalog so this stays in lockstep.
      local ver
      ver=$(yq -r ".versions[0]" "${WORKSPACE}/catalog/extensions/${name}.yaml")
      tag="${ver}-${PHP_VERSION}-nts-linux-${ARCH}"
      ref="${REGISTRY}/php-ext-${name}:${tag}"
      dest="$WORK/bundles/ext/${name}"
      ;;
    *) echo "pull_bundle: unknown kind $kind" >&2; return 1 ;;
  esac

  mkdir -p "$dest"
  local scratch="$WORK/.pull-${kind}-${name:-core}"
  rm -rf "$scratch"
  mkdir -p "$scratch"
  ( cd "$scratch" && oras pull --allow-path-traversal "$ref" >/dev/null )
  # oras writes /tmp/bundle.tar.zst + /tmp/meta.json because the bundle's OCI
  # layer paths reference those. Grab them before the next pull overwrites.
  [ -f /tmp/bundle.tar.zst ] || { echo "::error::oras produced no bundle for $ref" >&2; return 1; }
  tar --zstd -xf /tmp/bundle.tar.zst -C "$dest"
  cp /tmp/meta.json "$dest/meta.json" 2>/dev/null || true
}

# Build a mock /opt/buildrush layout from the bundles pulled above, mirroring
# what `phpup` produces at runtime.
compose_mock() {
  local fixture_exts="$1" mock="$WORK/mock-buildrush"
  rm -rf "$mock"
  mkdir -p "$mock/core"
  cp -R "$WORK/bundles/core/." "$mock/core/"

  if [ -z "$fixture_exts" ]; then return 0; fi
  IFS=',' read -r -a exts <<< "$fixture_exts"
  for ext in "${exts[@]}"; do
    ext="$(echo "$ext" | xargs)"  # trim whitespace
    [ -z "$ext" ] && continue
    mkdir -p "$mock/bundles/${ext}"
    cp -R "$WORK/bundles/ext/${ext}/." "$mock/bundles/${ext}/"
  done
}

# Assemble the apt-install line for a fixture: core deps + per-extension
# runtime_deps from the catalog.
apt_deps_for() {
  local fixture_exts="$1"
  local deps=("${CORE_RUNTIME_DEPS[@]}")
  if [ -n "$fixture_exts" ]; then
    IFS=',' read -r -a exts <<< "$fixture_exts"
    for ext in "${exts[@]}"; do
      ext="$(echo "$ext" | xargs)"
      [ -z "$ext" ] && continue
      # Extract runtime_deps.linux as one-per-line, skip empty list.
      while IFS= read -r dep; do
        [ -z "$dep" ] && continue
        deps+=("$dep")
      done < <(yq -r '.runtime_deps.linux // [] | .[]' "${WORKSPACE}/catalog/extensions/${ext}.yaml" 2>/dev/null || true)
    done
  fi
  # De-duplicate while preserving order.
  printf '%s\n' "${deps[@]}" | awk '!seen[$0]++'
}

# Build the docker image for a runner OS: base + apt deps layered in.
run_in_container() {
  local runner_os="$1" fixture_name="$2" fixture_exts="$3" mock="$WORK/mock-buildrush"

  local image
  case "$runner_os" in
    ubuntu-22.04) image="ubuntu:22.04" ;;
    ubuntu-24.04) image="ubuntu:24.04" ;;
    *) echo "::error::unknown runner_os: $runner_os" >&2; return 1 ;;
  esac

  local deps_line
  deps_line="$(apt_deps_for "$fixture_exts" | tr '\n' ' ')"

  # Always pin platform so docker doesn't pick a cached non-matching image.
  local docker_arch
  case "$ARCH" in
    x86_64) docker_arch="linux/amd64" ;;
    aarch64) docker_arch="linux/arm64" ;;
    *) echo "::error::unsupported ARCH: $ARCH" >&2; return 1 ;;
  esac
  local platform="--platform=$docker_arch"

  # Build ext-loading fragment. Empty fixture → just `php -v`. The ext path
  # contains a glob (no-debug-non-zts-<api>) that must be expanded inside the
  # container, not on the host, so embed it as a subshell that resolves once.
  local php_check
  if [ -z "$fixture_exts" ]; then
    php_check='php -v | head -1'
  else
    # Resolve each ext's .so path inside the container then run a single
    # php invocation with all extension=<path> flags.
    php_check=$(cat <<SCRIPT
set -e
EXT_ARGS=""
for ext in ${fixture_exts//,/ }; do
  so=\$(ls /opt/buildrush/bundles/\${ext}/usr/local/lib/php/extensions/no-debug-non-zts-*/\${ext}.so 2>/dev/null | head -1 || true)
  if [ -z "\$so" ]; then echo "[FAIL] \$ext: .so not found in extracted bundle"; exit 1; fi
  EXT_ARGS="\$EXT_ARGS -d extension=\$so"
done
for ext in ${fixture_exts//,/ }; do
  out=\$(php \$EXT_ARGS -r "echo extension_loaded('\$ext') ? 'OK' : 'FAIL';" 2>&1)
  if [ "\$out" != OK ]; then echo "[FAIL] \$ext: \$out"; exit 1; fi
  echo "[OK]   \$ext"
done
SCRIPT
)
  fi

  echo "--- $runner_os × $fixture_name ---"
  # shellcheck disable=SC2086
  if ! docker run --rm $platform -v "$mock:/opt/buildrush:ro" "$image" bash -c "
    set -e
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null 2>&1
    apt-get install -y -qq --no-install-recommends $deps_line >/dev/null
    ln -sf /opt/buildrush/core/usr/local/bin/php /usr/local/bin/php
    $php_check
  "; then
    echo "::error::$runner_os × $fixture_name FAILED"
    return 1
  fi
}

# Outer loop over arches so every arch gets its own pull + extract + run.
fail_count=0
for ARCH in $ARCHES; do
  echo
  echo "########## arch=${ARCH} ##########"

  # Reset per-arch bundle scratch so we don't cross-contaminate.
  rm -rf "$WORK/bundles"
  mkdir -p "$WORK/bundles"

  # 1. Pull core.
  echo "=== pulling php-core:${PHP_VERSION}-${ARCH} ==="
  pull_bundle core

  # 2. Pull every ext referenced by any fixture (deduped per arch).
  seen=()
  for entry in "${FIXTURES[@]}"; do
    exts="${entry#*|}"
    [ -z "$exts" ] && continue
    IFS=',' read -r -a es <<< "$exts"
    for ext in "${es[@]}"; do
      ext="$(echo "$ext" | xargs)"
      if ! printf '%s\n' "${seen[@]-}" | grep -qx "$ext"; then
        echo "=== pulling php-ext-${ext} ==="
        pull_bundle ext "$ext"
        seen+=("$ext")
      fi
    done
  done

  # 3. For every fixture × runner combination, compose the mock and run.
  for entry in "${FIXTURES[@]}"; do
    fname="${entry%%|*}"
    fexts="${entry#*|}"
    compose_mock "$fexts"
    for runner in "${RUNNERS[@]}"; do
      if ! run_in_container "$runner" "$fname" "$fexts"; then
        fail_count=$((fail_count + 1))
      fi
    done
  done
done

echo
if [ "$fail_count" -eq 0 ]; then
  echo "=== local-ci: ALL GREEN (arches=${ARCHES}, PHP $PHP_VERSION) ==="
  exit 0
fi
echo "::error::local-ci: $fail_count failure(s) across arches=${ARCHES}"
exit 1
