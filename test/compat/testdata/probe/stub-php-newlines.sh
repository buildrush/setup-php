#!/usr/bin/env bash
set -euo pipefail
# Stub that emits a newline in one ini value to exercise json_escape.
case "$*" in
  "-r echo PHP_VERSION;") echo -n "8.4.5" ;;
  "-r echo PHP_SAPI;") echo -n "cli" ;;
  "-r echo PHP_ZTS;") echo -n "0" ;;
  "-m") echo "[PHP Modules]"; echo "Core" ;;
  *)
    if [[ "${1:-}" == "-r" ]]; then
      case "${2:-}" in
        "echo ini_get('disable_functions');") printf "eval\nexec" ;;
        *) echo -n "" ;;
      esac
      exit 0
    fi
    exit 1
    ;;
esac
