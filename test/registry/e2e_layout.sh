#!/usr/bin/env bash
# End-to-end verification that `phpup install` can source bundles from a
# local OCI layout via the --registry flag (or PHPUP_REGISTRY env var).
#
# Flow:
#   1. Build phpup from the working tree.
#   2. Resolve a real php-core digest from bundles.lock.
#   3. Seed a fresh on-disk OCI layout from GHCR with `oras cp` (ONE-TIME
#      network access).
#   4. Run phpup with PHPUP_REGISTRY=oci-layout:<dir> — phpup must source
#      the bundle from the layout, never contacting GHCR again.
#   5. Assert the resolved PHP binary exists and executes.
#
# Intended as the manual gate for PR 1 of the local+CI unification rollout
# and a fixture that PR 4's CI rework will reuse.
#
# Requires (hard):
#   - oras  (https://oras.land) — copies the manifest into an OCI layout.
#   - jq    — reads bundles.lock.
#   - go    — builds phpup.
# Requires (soft):
#   - GHCR read access. Set GITHUB_TOKEN or GHCR_TOKEN if the image is
#     private or you are rate-limited by anonymous pulls. Otherwise
#     anonymous access to public buildrush images is sufficient.
#
# Platform: the published php-core bundles contain linux binaries, so the
# `php --version` smoke step only succeeds on a linux host. The script
# fails fast on non-linux with a clear message — wrap it in a linux
# container or run on CI.
#
# Usage:
#   ./test/registry/e2e_layout.sh
#
# Environment overrides:
#   WORK=<dir>       — reuse a scratch dir instead of a fresh mktemp -d
#                      (the dir is still wiped by the EXIT trap).
#   PHP_VERSION=8.4  — PHP major.minor to exercise (must have an entry in
#                      cmd/phpup/bundles.lock).
#   KEEP_WORK=1      — skip cleanup so you can inspect the layout on
#                      failure.

set -euo pipefail

PHP_VERSION="${PHP_VERSION:-8.4}"

require() {
    local cmd="$1"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "FAIL: missing required command: $cmd" >&2
        echo "      install it and re-run. See the header of this script for details." >&2
        exit 1
    fi
}

require oras
require jq
require go

# Resolve the phpup/bundles.lock arch key from the host.
HOST_UNAME="$(uname -m)"
case "$HOST_UNAME" in
    x86_64|amd64)   LOCK_ARCH="x86_64"; RUNNER_ARCH="X64" ;;
    aarch64|arm64)  LOCK_ARCH="aarch64"; RUNNER_ARCH="ARM64" ;;
    *)              echo "FAIL: unsupported host arch: $HOST_UNAME" >&2; exit 1 ;;
esac

# The published core bundle is linux-only. On darwin we can seed the
# layout fine, but the final `php --version` step would try to exec a
# linux ELF and fail. Bail early with a clear message.
HOST_OS="$(uname -s)"
if [[ "$HOST_OS" != "Linux" ]]; then
    echo "FAIL: this harness exercises linux php bundles and must run on a" >&2
    echo "      linux host. Detected: $HOST_OS. Wrap in a linux container or" >&2
    echo "      run on CI. (Docker-based packaging is Task 3 of PR 3.)" >&2
    exit 1
fi

REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
cd "$REPO_ROOT"

WORK="${WORK:-$(mktemp -d)}"
LAYOUT="$WORK/oci-layout"
BIN="$WORK/phpup"
INSTALL_DIR="$WORK/buildrush"

cleanup() {
    if [[ "${KEEP_WORK:-0}" == "1" ]]; then
        echo "KEEP_WORK=1: leaving $WORK in place for inspection" >&2
        return
    fi
    rm -rf "$WORK"
}
trap cleanup EXIT

echo "==> Work dir: $WORK"

echo "==> Regenerate embedded lockfile and build phpup"
make cmd/phpup/bundles.lock >/dev/null
go build -o "$BIN" ./cmd/phpup

echo "==> Resolve php-core digest for php:$PHP_VERSION:linux:$LOCK_ARCH:nts"
PHP_KEY="php:$PHP_VERSION:linux:$LOCK_ARCH:nts"
PHP_CORE_DIGEST="$(jq -r --arg k "$PHP_KEY" '.bundles[$k].digest // ""' cmd/phpup/bundles.lock)"
if [[ -z "$PHP_CORE_DIGEST" || "$PHP_CORE_DIGEST" == "null" ]]; then
    echo "FAIL: could not resolve $PHP_KEY digest from cmd/phpup/bundles.lock" >&2
    exit 1
fi
echo "    digest: $PHP_CORE_DIGEST"

echo "==> Seed local OCI layout from GHCR (one-time network access)"
mkdir -p "$LAYOUT"

# If a token is available, log oras in against ghcr.io so private/rate-
# limited pulls work. Anonymous pulls succeed for public images, so this
# step is best-effort.
ORAS_TOKEN="${GITHUB_TOKEN:-${GHCR_TOKEN:-}}"
if [[ -n "$ORAS_TOKEN" ]]; then
    echo "    authenticating to ghcr.io (token provided)"
    echo "$ORAS_TOKEN" | oras login ghcr.io --username "${GHCR_USERNAME:-oauth2}" --password-stdin >/dev/null
fi

# `oras cp` with `--to-oci-layout` pulls the manifest (by digest) into an
# OCI layout directory. The `:seed` tag is a hint that lands in the
# layout's index.json — the layout backend in internal/registry matches
# refs by digest, so the tag value doesn't have to match anything phpup
# expects.
oras cp --to-oci-layout \
    "ghcr.io/buildrush/php-core@$PHP_CORE_DIGEST" \
    "$LAYOUT:seed"

# Sanity-check the layout structure before handing off to phpup.
if [[ ! -s "$LAYOUT/index.json" ]]; then
    echo "FAIL: oras cp did not produce an index.json under $LAYOUT" >&2
    exit 1
fi

echo "==> Run phpup install against the local layout (offline after seed)"
export PHPUP_REGISTRY="oci-layout:$LAYOUT"
export BUILDRUSH_DIR="$INSTALL_DIR"
export INPUT_VERBOSE="true"
export RUNNER_OS="Linux"
export RUNNER_ARCH="$RUNNER_ARCH"

# Redirect env export to a file under $WORK so the script doesn't pollute
# the caller's shell (phpup appends PATH/PHPRC lines to $GITHUB_ENV when
# set, and falls back to a stdout echo otherwise; either is fine here).
export GITHUB_ENV="$WORK/github_env"
export GITHUB_PATH="$WORK/github_path"
export GITHUB_OUTPUT="$WORK/github_output"
: >"$GITHUB_ENV"
: >"$GITHUB_PATH"
: >"$GITHUB_OUTPUT"

# INPUT_PHP-VERSION uses a literal dash (GitHub Actions convention).
# Plain `export` can't declare names with dashes, so hand it to phpup via
# env's KEY=VALUE form.
env "INPUT_PHP-VERSION=$PHP_VERSION" "$BIN"

echo "==> Assert the resolved php binary exists and runs"
# See cmd/phpup/main.go:detectLayout — BinDir = <BUILDRUSH_DIR>/core/usr/local/bin.
PHP_BIN="$INSTALL_DIR/core/usr/local/bin/php"
if [[ ! -x "$PHP_BIN" ]]; then
    echo "FAIL: expected PHP binary missing at $PHP_BIN" >&2
    echo "---- $INSTALL_DIR layout ----" >&2
    find "$INSTALL_DIR" -maxdepth 5 >&2 || true
    exit 1
fi
"$PHP_BIN" --version

echo "==> PASS"
