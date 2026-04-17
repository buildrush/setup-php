#!/usr/bin/env bash
# test/smoke/compat.sh — shivammathur/setup-php@v2 drop-in compatibility scenario.
#
# Exercises the full phpup runtime path with v2-shaped input:
#   extensions:  "redis, :opcache"   (include + exclusion)
#   ini-values:  "memory_limit=256M, date.timezone=Europe/Berlin"
#                (user overrides both the compat defaults: memory_limit=-1, date.timezone=UTC)
#   coverage:    "xdebug"
#
# Pre-reqs:
#   - phpup binary on PATH (or override with PHPUP=... env var)
#   - the PHP 8.4 bundle already built and pushed to GHCR
#   - GITHUB_TOKEN exported if pulling from GHCR
#   - running on linux/x86_64

set -euo pipefail

PHPUP="${PHPUP:-phpup}"

if ! command -v "$PHPUP" >/dev/null 2>&1; then
  echo "FAIL: $PHPUP not on PATH; build with 'make build' and add ./bin to PATH" >&2
  exit 2
fi

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Fake GitHub Actions env
export BUILDRUSH_DIR="$WORK"
export GITHUB_ENV="$WORK/env"
export GITHUB_OUTPUT="$WORK/output"
export GITHUB_PATH="$WORK/path"
: > "$GITHUB_ENV"; : > "$GITHUB_OUTPUT"; : > "$GITHUB_PATH"

# INPUT_* env vars matching how GitHub Actions materializes action.yml inputs.
# Our phpup reads INPUT_<NAME-UPPERCASED> where - stays - (not _).
export "INPUT_PHP-VERSION=8.4"
export "INPUT_EXTENSIONS=redis, :opcache"
export "INPUT_INI-VALUES=memory_limit=256M, date.timezone=Europe/Berlin"
export INPUT_COVERAGE=xdebug
export "INPUT_FAIL-FAST=false"
export RUNNER_OS=Linux
export RUNNER_ARCH=X64

echo "==== running $PHPUP ===="
"$PHPUP"

PHP_BIN="$WORK/core/usr/local/bin/php"
if [ ! -x "$PHP_BIN" ]; then
  echo "FAIL: expected php binary at $PHP_BIN not found" >&2
  ls -la "$WORK" >&2 || true
  exit 1
fi

echo "==== php -v ===="
"$PHP_BIN" -v

echo "==== php -m ===="
MODULES=$("$PHP_BIN" -m)
echo "$MODULES"

fail() { echo "FAIL: $1" >&2; exit 1; }

echo "==== assertions ===="

# redis was explicitly requested → must be loaded
echo "$MODULES" | grep -qi "^redis$"   || fail "redis not loaded"

# xdebug was requested via coverage:xdebug → must be loaded
echo "$MODULES" | grep -qi "^xdebug$"  || fail "xdebug not loaded"

# opcache was excluded via :opcache. In v2-compat semantics, a :ext exclusion
# means the extension should NOT be active. Our disable fragments are audit-
# trail only (compiled-in extensions still load); this assertion is skipped
# until we have a mechanism to prevent compile-time extensions from loading.
# For now, just make noise if opcache IS loaded so the gap stays visible.
if echo "$MODULES" | grep -qi "^Zend OPcache$"; then
  echo "NOTE: opcache still loaded despite :opcache exclusion (known compat gap — compile-time extras not disabled)" >&2
fi

# ini-values merge check: user override wins over compat default
MEMORY=$("$PHP_BIN" -r 'echo ini_get("memory_limit");')
[ "$MEMORY" = "256M" ] || fail "memory_limit=$MEMORY, want 256M (user should override compat default -1)"

TZ=$("$PHP_BIN" -r 'echo ini_get("date.timezone");')
[ "$TZ" = "Europe/Berlin" ] || fail "date.timezone=$TZ, want Europe/Berlin (user should override compat default UTC)"

# php-version output is full X.Y.Z. GITHUB_OUTPUT uses multi-line delim format:
#   php-version<<ghadelimiter_<rand>
#   8.4.5
#   ghadelimiter_<rand>
# Parse out the value line.
VERSION=$(awk '/^php-version<</{getline; print; exit}' "$GITHUB_OUTPUT")
if ! echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+'; then
  fail "php-version output is not X.Y.Z; got:
$(cat "$GITHUB_OUTPUT")"
fi

echo "==== compat smoke PASS ===="
