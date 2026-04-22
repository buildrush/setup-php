#!/usr/bin/env bats

setup() {
    TMP="$(mktemp -d)"
    OUT="$TMP/hermetic"; mkdir -p "$OUT"
    # Pick a real system binary with predictable dependencies.
    TARGET="$TMP/lsbin"
    cp /bin/ls "$TARGET"
    # Give the target an rpath pointing at OUT so the rpath assertion passes.
    if ! command -v patchelf >/dev/null 2>&1; then
        skip "patchelf not available"
    fi
    patchelf --set-rpath "$OUT" "$TARGET"
}

teardown() {
    rm -rf "$TMP"
}

@test "empty glob list is no-op and emits empty captured array" {
    run ./builders/common/capture-hermetic-libs.sh --target "$TARGET" --globs '' --output "$OUT"
    [ "$status" -eq 0 ]
    [ "$(echo "$output" | jq -r '.captured | length')" = "0" ]
}

@test "matching glob captures libc-derived names" {
    run ./builders/common/capture-hermetic-libs.sh --target "$TARGET" --globs 'libc.so.*' --output "$OUT"
    [ "$status" -eq 0 ]
    found=$(find "$OUT" -name 'libc.so.*' | wc -l)
    [ "$found" -gt 0 ]
    [ "$(echo "$output" | jq -r '.captured[0]' | head -c 4)" = "libc" ]
}

@test "non-matching glob fails without --allow-missing-glob" {
    run ./builders/common/capture-hermetic-libs.sh --target "$TARGET" --globs 'libdoesnotexist.so.*' --output "$OUT"
    [ "$status" -ne 0 ]
    echo "$output" | grep -q "glob.*no matches"
}

@test "non-matching glob succeeds with --allow-missing-glob" {
    run ./builders/common/capture-hermetic-libs.sh --target "$TARGET" --globs 'libdoesnotexist.so.*' --output "$OUT" --allow-missing-glob
    [ "$status" -eq 0 ]
    [ "$(echo "$output" | jq -r '.captured | length')" = "0" ]
}

@test "target lacking rpath at output dir fails" {
    patchelf --set-rpath "/tmp/other" "$TARGET"
    run ./builders/common/capture-hermetic-libs.sh --target "$TARGET" --globs 'libc.so.*' --output "$OUT"
    [ "$status" -ne 0 ]
    echo "$output" | grep -q "rpath"
}
