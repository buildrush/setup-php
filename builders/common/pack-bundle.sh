#!/usr/bin/env bash
set -euo pipefail

# Pack a directory into a zstd-compressed tarball with metadata sidecar.
# Usage: pack-bundle.sh <source-dir> <output-path>

SOURCE_DIR="${1:?Usage: pack-bundle.sh <source-dir> <output-path>}"
OUTPUT_PATH="${2:?Usage: pack-bundle.sh <source-dir> <output-path>}"

# Create zstd-compressed tarball
tar --zstd -cf "$OUTPUT_PATH" -C "$SOURCE_DIR" .

# Compute digest
DIGEST=$(sha256sum "$OUTPUT_PATH" | awk '{print $1}')
echo "sha256:$DIGEST" > "${OUTPUT_PATH}.sha256"

# Generate metadata sidecar
cat > "$(dirname "$OUTPUT_PATH")/meta.json" <<METAEOF
{
  "build_timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "digest": "sha256:$DIGEST",
  "builder_versions": {
    "gcc": "$(gcc --version 2>/dev/null | head -1 || echo 'N/A')",
    "autoconf": "$(autoconf --version 2>/dev/null | head -1 || echo 'N/A')"
  }
}
METAEOF

echo "$DIGEST"
