#!/usr/bin/env bash
set -euo pipefail

# Pack a directory into a zstd-compressed tarball with metadata sidecar.
# Usage: pack-bundle.sh <kind> <source-dir> <output-path> [capture-json]
#   kind: "php-core" | "php-ext" | "php-tool"
#   capture-json (optional): path to a JSON file produced by
#       capture-hermetic-libs.sh — {"captured": [...], "skipped_system": [...]}

KIND="${1:?Usage: pack-bundle.sh <kind> <source-dir> <output-path> [capture-json]}"
SOURCE_DIR="${2:?Usage: pack-bundle.sh <kind> <source-dir> <output-path> [capture-json]}"
OUTPUT_PATH="${3:?Usage: pack-bundle.sh <kind> <source-dir> <output-path> [capture-json]}"
CAPTURE_JSON="${4:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "${SCRIPT_DIR}/bundle-schema-version.env"
# shellcheck disable=SC1091
. "${SCRIPT_DIR}/builder-os.env"
case "$KIND" in
  php-core) SCHEMA_VERSION="${BUNDLE_SCHEMA_PHP_CORE}" ;;
  php-ext)  SCHEMA_VERSION="${BUNDLE_SCHEMA_PHP_EXT}"  ;;
  *) echo "pack-bundle: unknown kind ${KIND}" >&2; exit 1 ;;
esac

tar --zstd -cf "$OUTPUT_PATH" -C "$SOURCE_DIR" .

DIGEST=$(sha256sum "$OUTPUT_PATH" | awk '{print $1}')
echo "sha256:$DIGEST" > "${OUTPUT_PATH}.sha256"

# Extract captured[] from capture JSON if provided; default to empty array.
if [ -n "$CAPTURE_JSON" ] && [ -f "$CAPTURE_JSON" ]; then
  HERMETIC_JSON=$(jq -c '.captured // []' "$CAPTURE_JSON")
else
  HERMETIC_JSON='[]'
fi

cat > "$(dirname "$OUTPUT_PATH")/meta.json" <<METAEOF
{
  "schema_version": ${SCHEMA_VERSION},
  "kind": "${KIND}",
  "build_timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "digest": "sha256:$DIGEST",
  "builder_os": "${BUILDER_OS}",
  "hermetic_libs": ${HERMETIC_JSON},
  "builder_versions": {
    "gcc": "$(gcc --version 2>/dev/null | head -1 || echo 'N/A')",
    "autoconf": "$(autoconf --version 2>/dev/null | head -1 || echo 'N/A')"
  }
}
METAEOF

echo "$DIGEST"
