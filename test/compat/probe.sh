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
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\t'/\\t}"
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

# env_delta: names only. Compare current env names against the env-before snapshot,
# filtering out bash-internal/auto-set vars so the delta reflects what setup-php
# added, not noise from the shell.
bash_internal_vars='^(PATH|PWD|OLDPWD|SHLVL|IFS|PS1|PS2|PS4|OPTIND|BASH|BASH_VERSION|BASH_VERSINFO|BASH_ENV|BASHOPTS|SHELLOPTS|HOSTNAME|HOSTTYPE|OSTYPE|MACHTYPE|UID|EUID|PPID|LINENO|RANDOM|SECONDS|_)$'

current_env_names="$(env | awk -F= '{print $1}' | LC_ALL=C sort -u | grep -vE "$bash_internal_vars" || true)"
before_env_names="$(awk -F= '{print $1}' "$env_before" | LC_ALL=C sort -u)"
env_added="$(comm -23 <(printf "%s\n" "$current_env_names") <(printf "%s\n" "$before_env_names"))"
env_delta_csv="$(
  printf "%s" "$env_added" \
    | awk 'BEGIN { first=1 } NF { if (first) { printf "\"%s\"", $1; first=0 } else { printf ",\"%s\"", $1 } }'
)"

# path_additions: diff current PATH entries vs path-before, normalize PHP tool-cache paths.
normalize_path_entry() {
  # Replace .../PHP/X.Y(.Z)?/<suffix> with <PHP_ROOT>/<suffix>
  echo "$1" | sed -E 's#^.*/PHP/[0-9]+\.[0-9]+(\.[0-9]+)?/#<PHP_ROOT>/#'
}

before_entries="$(tr ':' '\n' < "$path_before" | LC_ALL=C sort -u)"
current_entries="$(printf "%s" "$PATH" | tr ':' '\n' | LC_ALL=C sort -u)"
path_added="$(comm -23 <(printf "%s\n" "$current_entries") <(printf "%s\n" "$before_entries"))"

path_additions_csv="$(
  while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue
    normalize_path_entry "$entry"
  done <<<"$path_added" \
  | LC_ALL=C sort -u \
  | awk 'BEGIN { first=1 } NF { if (first) { printf "\"%s\"", $1; first=0 } else { printf ",\"%s\"", $1 } }'
)"

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
