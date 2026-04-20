#!/usr/bin/env bash
set -euo pipefail
# Fake php: emits canned responses based on CLI args.
case "$*" in
  "-r echo PHP_VERSION;")
    echo -n "8.4.5"
    ;;
  "-r echo PHP_SAPI;")
    echo -n "cli"
    ;;
  "-r echo PHP_ZTS;")
    echo -n "0"
    ;;
  "-m")
    cat <<'EOF'
[PHP Modules]
Core
curl
Ctype

[Zend Modules]
Zend OPcache
EOF
    ;;
  *)
    if [[ "${1:-}" == "-r" ]]; then
      case "${2:-}" in
        "echo ini_get('memory_limit');") echo -n "128M" ;;
        "echo ini_get('date.timezone');") echo -n "UTC" ;;
        "echo ini_get('opcache.enable');") echo -n "1" ;;
        *) echo -n "" ;;  # unknown key → empty (matches real ini_get of unset key)
      esac
      exit 0
    fi
    echo "stub-php: unknown args: $*" >&2
    exit 1
    ;;
esac
