#!/usr/bin/env bats

setup() {
    TMP="$(mktemp -d)"
    SRC="$TMP/src"; mkdir -p "$SRC"
    echo "hello" > "$SRC/hello.txt"
    CAPTURE_JSON="$TMP/capture.json"
    echo '{"captured":["libicui18n.so.70","libicuuc.so.70"],"skipped_system":["libc.so.6"]}' > "$CAPTURE_JSON"
    OUT="$TMP/bundle.tar.zst"
}

teardown() {
    rm -rf "$TMP"
}

@test "meta.json contains hermetic_libs and builder_os when capture json given" {
    run ./builders/common/pack-bundle.sh php-core "$SRC" "$OUT" "$CAPTURE_JSON"
    [ "$status" -eq 0 ]
    META="$(dirname "$OUT")/meta.json"
    [ -f "$META" ]
    run jq -r '.hermetic_libs | join(",")' "$META"
    [ "$output" = "libicui18n.so.70,libicuuc.so.70" ]
    run jq -r '.builder_os' "$META"
    [ "$output" = "ubuntu-22.04" ]
    run jq -r '.schema_version' "$META"
    [ "$output" = "3" ]
}

@test "meta.json has empty hermetic_libs when no capture json given" {
    run ./builders/common/pack-bundle.sh php-core "$SRC" "$OUT"
    [ "$status" -eq 0 ]
    META="$(dirname "$OUT")/meta.json"
    run jq -r '.hermetic_libs | length' "$META"
    [ "$output" = "0" ]
}
