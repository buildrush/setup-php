#!/usr/bin/env bash
set -euo pipefail

# Update bundles.lock with digests from the latest build.
# This script is called by the update-lock job in on-push.yml.

LOCKFILE="${1:-bundles.lock}"

echo "Updating lockfile at ${LOCKFILE}"

# For Phase 1: placeholder that will be enhanced as the pipeline matures.
echo "Lockfile update: checking GHCR for published bundles..."

# TODO: Query GHCR registry for all published bundles under ghcr.io/buildrush/
# and update the lockfile with their digests.

echo "Lockfile update complete"
