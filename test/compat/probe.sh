#!/usr/bin/env bash
# probe.sh — emit a normalized JSON snapshot of the PHP environment that
# shivammathur/setup-php (or our setup-php) has produced. Keep output
# byte-deterministic.
#
# Usage:
#   probe.sh <out.json> <env-before-file> <path-before-file> <ini-keys-file>

set -euo pipefail

out="${1:?out.json path required}"
env_before="${2:?env-before path required}"
path_before="${3:?path-before path required}"
ini_keys_file="${4:?ini-keys path required}"

php_version="$(php -r 'echo PHP_VERSION;')"
sapi="$(php -r 'echo PHP_SAPI;')"
zts_raw="$(php -r 'echo PHP_ZTS;')"
if [[ "$zts_raw" == "1" ]]; then zts="true"; else zts="false"; fi

# Extensions: strip bracket headers and empty lines, lowercase, sort.
extensions_csv="$(
  php -m \
    | grep -v '^\[' \
    | sed '/^[[:space:]]*$/d' \
    | tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C sort \
    | awk 'BEGIN { first=1 } { if (first) { printf "\"%s\"", $0; first=0 } else { printf ",\"%s\"", $0 } }'
)"

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf "%s" "$s"
}

# INI: for each curated key (sorted+deduped), ask php for ini_get, build JSON map.
ini_pairs=""
while IFS= read -r key; do
  [[ -z "$key" ]] && continue
  raw="$(php -r "echo ini_get('${key}');" || true)"
  if [[ "$raw" == "false" ]]; then raw=""; fi
  esc="$(json_escape "$raw")"
  if [[ -n "$ini_pairs" ]]; then ini_pairs+=","; fi
  ini_pairs+="\"${key}\":\"${esc}\""
done < <(LC_ALL=C sort -u "$ini_keys_file")

# env_delta and path_additions: not wired in this task — empty arrays.
env_delta_csv=""
path_additions_csv=""

cat >"$out" <<JSON
{
  "php_version": "$(json_escape "$php_version")",
  "sapi": "$(json_escape "$sapi")",
  "zts": ${zts},
  "extensions": [${extensions_csv}],
  "ini": {${ini_pairs}},
  "env_delta": [${env_delta_csv}],
  "path_additions": [${path_additions_csv}]
}
JSON

# Silence unused-for-now inputs.
: "${env_before}" "${path_before}"
