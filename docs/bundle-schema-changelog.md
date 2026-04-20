# Bundle Schema Changelog

Tracks per-kind `schema_version` bumps in `builders/common/bundle-schema-version.env`. One entry per bump. Bump when adding/removing a file or directory the runtime asserts on, or changing on-disk layout the runtime depends on. Do not bump for upstream-source-only changes.

## `php-core`

### v2 (2026-04-20)

Bundle layout now includes `share/php/ini/php.ini-production` and `share/php/ini/php.ini-development`. Runtime (`internal/compose`) asserts these files exist when `ini-file` is `production` or `development`. Introduced in PR #28 (Phase 2 compat closeout); retrofitted to schema v2 in PR-β.1.

### v1 (initial)

Bundle contains `bin/php`, `bin/php-cgi`, `bin/phpize`, `bin/php-config`, `bin/pecl`, `bin/pear`, core extensions, `lib/libphp*`, basic ini templates.

## `php-ext`

### v1 (initial)

Bundle contains a single `.so` plus any statically-linked runtime libraries not bundled into the binary itself.
