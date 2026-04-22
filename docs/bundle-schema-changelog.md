# Bundle Schema Changelog

Tracks per-kind `schema_version` bumps in `builders/common/bundle-schema-version.env`. One entry per bump. Bump when adding/removing a file or directory the runtime asserts on, or changing on-disk layout the runtime depends on. Do not bump for upstream-source-only changes.

## v3 (2026-04-22) — hermetic library capture

- Adds `usr/local/lib/hermetic/` (core) and `hermetic/` (ext) directories, populated at build time from the catalog's `hermetic_libs` glob list.
- Adds `meta.json.hermetic_libs` (resolved filenames, not globs) and `meta.json.builder_os`.
- Binaries in the bundle carry `DT_RUNPATH` / `DT_RPATH` entries pointing into the hermetic dirs; `compose` does not set `LD_LIBRARY_PATH`.
- Rationale: cross-OS runtime compatibility (jammy builds running on noble and future Ubuntu releases). Replaces the slice-D approach of glibc-forward-compat, which did not hold for versioned `.so` dependencies like ICU and ImageMagick.
- **`MinBundleSchema` intentionally stays at v2 (core) / v1 (ext):** nothing in the runtime asserts on v3-specific fields. `hermetic_libs` in `meta.json` is informational (for `phpup doctor` and `cmd/hermetic-audit`); the actual hermetic loading works via rpath regardless of the sidecar. This avoids a chicken-and-egg problem where bumping `MinBundleSchema` in the same PR that produces the first v3 bundles would make CI's `ours:` jobs reject the still-v2 bundles currently on GHCR. A future slice that adds a runtime feature depending on a v3-only field bumps `MinBundleSchema` then.

## `php-core`

### v2 (2026-04-20)

Bundle layout now includes `share/php/ini/php.ini-production` and `share/php/ini/php.ini-development`. Runtime (`internal/compose`) asserts these files exist when `ini-file` is `production` or `development`. Introduced in PR #28 (Phase 2 compat closeout); retrofitted to schema v2 in PR-β.1.

### v1 (initial)

Bundle contains `bin/php`, `bin/php-cgi`, `bin/phpize`, `bin/php-config`, `bin/pecl`, `bin/pear`, core extensions, `lib/libphp*`, basic ini templates.

## `php-ext`

### v1 (initial)

Bundle contains a single `.so` plus any statically-linked runtime libraries not bundled into the binary itself.
