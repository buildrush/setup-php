#!/usr/bin/env bash
set -euo pipefail

# Capture shared libraries matching catalog globs into a bundle's hermetic dir.
#
# Usage:
#   capture-hermetic-libs.sh --target <binary> --globs <g1,g2,...> \
#       --output <dir> [--allow-missing-glob]
#
# Emits JSON to stdout:
#   {"captured": ["libfoo.so.1", ...], "skipped_system": ["libc.so.6", ...]}
#
# Exit non-zero on: missing required args, zero-match glob (unless allowed),
# target binary lacking an rpath entry pointing at --output, copy error.

TARGET=""
GLOBS_CSV=""
OUTPUT=""
ALLOW_MISSING=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET="$2"; shift 2 ;;
    --globs) GLOBS_CSV="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    --allow-missing-glob) ALLOW_MISSING=true; shift ;;
    *) echo "::error::capture-hermetic-libs: unknown arg: $1" >&2; exit 2 ;;
  esac
done

[ -n "$TARGET" ] || { echo "::error::--target required" >&2; exit 2; }
[ -n "$OUTPUT" ] || { echo "::error::--output required" >&2; exit 2; }
[ -f "$TARGET" ] || { echo "::error::target not a file: $TARGET" >&2; exit 2; }
mkdir -p "$OUTPUT"

# Split globs by comma into array.
IFS=',' read -r -a GLOBS <<< "$GLOBS_CSV"

# If no globs, exit clean with empty JSON.
if [ -z "$GLOBS_CSV" ] || [ "${#GLOBS[@]}" -eq 0 ]; then
  echo '{"captured":[],"skipped_system":[]}'
  exit 0
fi

# Collect ldd output for a given binary as "name path" lines, skipping vDSO and
# "statically linked".
ldd_pairs() {
    local bin="$1"
    ldd "$bin" 2>/dev/null | awk '
        /statically linked/ { next }
        /linux-vdso/ { next }
        /ld-linux/ && $0 !~ /=>/ { print $1 " " $1; next }
        /=>/ {
            name=$1; path=$3;
            if (path != "" && path != "(") print name " " path;
        }
    '
}

matches_any_glob() {
    local name="$1"
    local g
    for g in "${GLOBS[@]}"; do
        # shellcheck disable=SC2053
        [[ "$name" == $g ]] && return 0
    done
    return 1
}

declare -A CAPTURED_PATHS=()
declare -A SKIPPED=()
declare -A MATCHED_GLOBS=()

# First wave.
while read -r name path; do
    [ -z "$name" ] && continue
    if matches_any_glob "$name"; then
        CAPTURED_PATHS[$name]="$path"
        for g in "${GLOBS[@]}"; do
            # shellcheck disable=SC2053
            if [[ "$name" == $g ]]; then MATCHED_GLOBS[$g]=1; fi
        done
    else
        SKIPPED[$name]=1
    fi
done < <(ldd_pairs "$TARGET")

# Transitive walk.
PREV_COUNT=-1
while [ "${#CAPTURED_PATHS[@]}" -ne "$PREV_COUNT" ]; do
    PREV_COUNT="${#CAPTURED_PATHS[@]}"
    for nm in "${!CAPTURED_PATHS[@]}"; do
        p="${CAPTURED_PATHS[$nm]}"
        while read -r tname tpath; do
            [ -z "$tname" ] && continue
            if [ -n "${CAPTURED_PATHS[$tname]:-}" ]; then continue; fi
            if matches_any_glob "$tname"; then
                CAPTURED_PATHS[$tname]="$tpath"
                for g in "${GLOBS[@]}"; do
                    # shellcheck disable=SC2053
                    if [[ "$tname" == $g ]]; then MATCHED_GLOBS[$g]=1; fi
                done
            else
                SKIPPED[$tname]=1
            fi
        done < <(ldd_pairs "$p")
    done
done

# Unmatched-glob check.
for g in "${GLOBS[@]}"; do
    if [ -z "${MATCHED_GLOBS[$g]:-}" ]; then
        if ! $ALLOW_MISSING; then
            echo "::error::capture-hermetic-libs: glob '$g' has no matches in ldd output of $TARGET" >&2
            exit 1
        fi
    fi
done

# Copy captured files, dereferencing symlinks so the final basename lands in
# $OUTPUT and the linker finds it by name.
for nm in "${!CAPTURED_PATHS[@]}"; do
    p="${CAPTURED_PATHS[$nm]}"
    dst="$OUTPUT/$nm"
    if [ -e "$dst" ]; then
        if ! cmp -s "$p" "$dst"; then
            echo "::error::capture-hermetic-libs: $dst exists with different content than source $p" >&2
            exit 1
        fi
        continue
    fi
    cp -L "$p" "$dst"
done

# Assert target declares at least one DT_RUNPATH or DT_RPATH entry. We do NOT
# try to resolve $ORIGIN and compare against $OUTPUT: the target may be at a
# different path at build time than at runtime (e.g. ext .so compiled under
# usr/local/lib/php/extensions/... but moved to bundle root at compose time).
# Callers that want the rpath to contain specific directories should assert
# that explicitly; here we only verify the target has any rpath at all so an
# accidentally-dropped patchelf invocation fails loudly.
if ! readelf -d "$TARGET" 2>/dev/null | grep -Eq '\b(RUN)?PATH\b'; then
    echo "::error::capture-hermetic-libs: $TARGET has no DT_RUNPATH/DT_RPATH — builder must apply rpath via patchelf or -Wl,-rpath before capture" >&2
    exit 1
fi

captured_json=$(printf '%s\n' "${!CAPTURED_PATHS[@]}" | sort | jq -R . | jq -s .)
skipped_json=$(printf '%s\n' "${!SKIPPED[@]}" | sort | jq -R . | jq -s .)
jq -c -n --argjson c "$captured_json" --argjson s "$skipped_json" \
    '{captured:$c, skipped_system:$s}'
