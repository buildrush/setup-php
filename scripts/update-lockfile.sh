#!/usr/bin/env bash
set -euo pipefail

# Update bundles.lock with digests from published GHCR bundles.
# This script is called by the update-lock job in on-push.yml.

LOCKFILE="${1:-bundles.lock}"
REGISTRY="${REGISTRY:-ghcr.io/buildrush}"
CATALOG_DIR="${CATALOG_DIR:-catalog}"

echo "Updating lockfile at ${LOCKFILE}"

# Login to GHCR if token available
if [ -n "${GHCR_TOKEN:-}" ]; then
  echo "$GHCR_TOKEN" | oras login ghcr.io -u token --password-stdin
fi

# Start building the bundles JSON object
BUNDLES="{}"

add_bundle() {
  local key="$1" digest="$2"
  BUNDLES=$(echo "$BUNDLES" | python3 -c "import sys,json; d=json.load(sys.stdin); d['${key}']='${digest}'; json.dump(d,sys.stdout)")
}

# Query PHP core bundles from catalog
PHP_VERSIONS=$(python3 -c "import yaml; d=yaml.safe_load(open('${CATALOG_DIR}/php.yaml')); [print(v) for v in d['versions']]")
OS_LIST=$(python3 -c "import yaml; d=yaml.safe_load(open('${CATALOG_DIR}/php.yaml')); [print(v) for v in d['abi_matrix']['os']]")
ARCH_LIST=$(python3 -c "import yaml; d=yaml.safe_load(open('${CATALOG_DIR}/php.yaml')); [print(v) for v in d['abi_matrix']['arch']]")
TS_LIST=$(python3 -c "import yaml; d=yaml.safe_load(open('${CATALOG_DIR}/php.yaml')); [print(v) for v in d['abi_matrix']['ts']]")

for ver in $PHP_VERSIONS; do
  for os in $OS_LIST; do
    for arch in $ARCH_LIST; do
      for ts in $TS_LIST; do
        TAG="${ver}-${os}-${arch}-${ts}"
        REF="${REGISTRY}/php-core:${TAG}"
        echo -n "Checking ${REF}... "

        # Get the manifest digest (used for content-addressed OCI pulls).
        DIGEST=$(oras manifest fetch "${REF}" --descriptor 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('digest',''))" 2>/dev/null) || true
        if [ -n "$DIGEST" ]; then
          # Key by the catalog version (e.g. "8.4"), matching the resolver's
          # lookup. Patch version info lives in the bundle manifest.
          KEY="php:${ver}:${os}:${arch}:${ts}"
          add_bundle "$KEY" "$DIGEST"
          echo "${KEY} = ${DIGEST}"
        else
          echo "not found"
        fi
      done
    done
  done
done

# Query extension bundles
for ext_file in "${CATALOG_DIR}"/extensions/*.yaml; do
  EXT_KIND=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); print(d.get('kind',''))")

  # Skip bundled extensions
  if [ "$EXT_KIND" = "bundled" ]; then
    continue
  fi

  EXT_NAME=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); print(d['name'])")
  EXT_VERSIONS=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); [print(v) for v in d.get('versions',[])]")
  EXT_PHP=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); [print(v) for v in d['abi_matrix']['php']]")
  EXT_OS=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); [print(v) for v in d['abi_matrix']['os']]")
  EXT_ARCH=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); [print(v) for v in d['abi_matrix']['arch']]")
  EXT_TS=$(python3 -c "import yaml; d=yaml.safe_load(open('${ext_file}')); [print(v) for v in d['abi_matrix']['ts']]")

  for ext_ver in $EXT_VERSIONS; do
    for php in $EXT_PHP; do
      for os in $EXT_OS; do
        for arch in $EXT_ARCH; do
          for ts in $EXT_TS; do
            TAG="${ext_ver}-${php}-${ts}-${os}-${arch}"
            REF="${REGISTRY}/php-ext-${EXT_NAME}:${TAG}"
            echo -n "Checking ${REF}... "

            # Get the manifest digest (used for content-addressed OCI pulls).
        DIGEST=$(oras manifest fetch "${REF}" --descriptor 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('digest',''))" 2>/dev/null) || true
            if [ -n "$DIGEST" ]; then
              KEY="ext:${EXT_NAME}:${ext_ver}:${php}:${os}:${arch}:${ts}"
              add_bundle "$KEY" "$DIGEST"
              echo "${KEY} = ${DIGEST}"
            else
              echo "not found"
            fi
          done
        done
      done
    done
  done
done

# Write lockfile
BUNDLE_COUNT=$(echo "$BUNDLES" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
python3 -c "
import json, datetime
bundles = json.loads('''${BUNDLES}''')
lockfile = {
    'schema_version': 1,
    'generated_at': datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ'),
    'bundles': dict(sorted(bundles.items()))
}
with open('${LOCKFILE}', 'w') as f:
    json.dump(lockfile, f, indent=2)
    f.write('\n')
"

echo "Lockfile updated with ${BUNDLE_COUNT} bundles"
cat "$LOCKFILE"
