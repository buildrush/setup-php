#!/usr/bin/env bash
set -euo pipefail
# test/smoke/local-ci.sh — DEPRECATED.
#
# This script used to orchestrate docker runs + oras pulls + fixture loops
# directly. PR 3 of the local+CI unification rollout moves that logic into
# `phpup test` (Go). This shim keeps the old invocation working by
# forwarding to `make ci-cell` with the same semantics (jammy + noble for
# the chosen --arch + --php).
#
# Prefer calling `make ci-cell OS=... ARCH=... PHP=...` or `make ci` directly
# in new work. This wrapper will be deleted in a future PR.

echo "NOTE: test/smoke/local-ci.sh is deprecated. Use 'make ci-cell OS=... ARCH=... PHP=...' or 'make ci' instead." >&2

PHP=${PHP:-8.4}
ARCH=${ARCH:-x86_64}
while [ $# -gt 0 ]; do
    case "$1" in
        --php)      PHP="$2"; shift 2 ;;
        --arch)     ARCH="$2"; shift 2 ;;
        --keep)     export KEEP_WORK=1; shift ;;
        -h|--help)
            cat <<EOF
Usage: $0 [--php <version>] [--arch <x86_64|aarch64>] [--keep]

DEPRECATED shim forwarding to: make ci-cell OS={jammy,noble} ARCH=<arch> PHP=<version>
EOF
            exit 0
            ;;
        *)
            echo "NOTE: ignoring unknown arg '$1' (forwarding to make ci-cell)" >&2
            shift
            ;;
    esac
done

REPO_ROOT="$(git rev-parse --show-toplevel)"
for os in jammy noble; do
    echo "==> (wrapper) make -C $REPO_ROOT ci-cell OS=$os ARCH=$ARCH PHP=$PHP"
    make -C "$REPO_ROOT" ci-cell OS="$os" ARCH="$ARCH" PHP="$PHP"
done
