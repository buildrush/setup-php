# Phase 2 — Compat Closeout: `ini-file`, Injected Defaults, `coverage` Side-Effects

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-20
**Supersedes:** Nothing (slice #2 of the Phase 2 umbrella)
**Target release:** `v0.2.0-alpha.2`

## 1. Summary

This slice closes the compat debt remaining from the Phase 2 compat-first slice (`2026-04-17-phase2-compat-slice-design.md`) that the compat harness currently masks via the deviations allowlist. Three pieces of v2 behavior are implemented end-to-end:

1. **`ini-file` input** (`production` default / `development` / `none`), currently parsed but a no-op + warn.
2. **v2's injected ini defaults** (`memory_limit=-1`, `date.timezone=UTC`, plus the PHP 8.x opcache/JIT block), currently allowlisted as `ignore`.
3. **`coverage` input side-effects** (compat-matrix §5.6/5.7/5.8 — auto-install driver, auto-disable the other, inject driver ini), currently unimplemented and invisible to the harness because no fixture exercises `coverage: pcov`.

Two new harness fixtures (`coverage-pcov`, `ini-file-development`) prove the work and prevent regression. Nine ini-path allowlist entries in `docs/compat-matrix.md` are deleted as part of this slice — the harness starts enforcing what the compat-slice spec always promised it would.

The slice ships **before** matrix expansion (versions, top-50, aarch64, ubuntu-22.04) so every slice after this one builds against a harness that actually detects ini-layer drift.

## 2. Goals & non-goals

### Goals
- `ini-file: production` copies PHP's `php.ini-production` over the effective `php.ini`; `development` copies `php.ini-development`; `none` writes an empty `php.ini`. Each mode lands on the effective PHP environment exactly the way v2 lands it (end-state ini values match).
- `internal/compat.DefaultIniValues` returns v2's CI-injected defaults per PHP version, and `internal/compose` layers them onto the effective config.
- `internal/compat.XdebugIniFragment` replaces our current `internal/compose`-hosted xdebug ini injection — drops the spurious `xdebug.start_with_request=default` we emit today.
- `coverage: xdebug` auto-adds `xdebug`, disables `pcov`; `coverage: pcov` auto-adds `pcov`, disables `xdebug`, injects `pcov.enabled=1`; `coverage: none` disables both.
- Two new harness fixtures cover `coverage: pcov` and `ini-file: development`.
- The allowlist in `docs/compat-matrix.md` shrinks accordingly; the disposition table in §5 of that doc marks 5.6/5.7/5.8/5.17 as implemented.

### Non-goals
- PHP versions other than 8.4 (Phase 2 slice #3).
- aarch64 (slice #5).
- ubuntu-22.04 (slice #6).
- Top-50 extensions (slice #4).
- L5 quirks marked "document" or "follow-up" in compat-matrix §5 — including L5.5 (Composer auth env), L5.11 (`update: true`), L5.12 (`debug: true`), L5.13 (`phpts: ts|zts`), L5.16 (`pre-installed`), L5.21 (PECL fallback). Each of those keeps its current disposition.
- `tools:` input parity — Phase 3.
- Environment-var exports driven by Composer auto-install (§5.3) — Phase 3, when tools land.
- The remaining four opcache allowlist entries (`ini.opcache.enable_cli`, `…memory_consumption`, `…revalidate_freq`, `…validate_timestamps`) — these come from PHP's compiled-in defaults via `php.ini-production`, not from v2's injected layer, and are expected to match automatically once 2A lands. The allowlist entries stay until the harness run after implementation confirms match, at which point they are deleted in the same PR.

## 3. Source of truth

Same pinning as slice #1 (`docs/compat-matrix.md`):

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` |

Compat-matrix §1.1 note (ini-file aliases), §2.1–§2.5 (default ini values and base-file selection), §5.6–§5.8 (coverage side-effects), §5.17 (`ini-file: none` truncation) are the sections this slice implements.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `builders/linux/build-php.sh` | Capture `php.ini-production` and `php.ini-development` from the PHP source tarball; place both under `share/php/ini/` in the core bundle. |
| `internal/compat` | Add `DefaultIniValues(phpVersion) map[string]string`, `XdebugIniFragment(phpVersion) map[string]string`, `BaseIniFileName(iniFile) string`. Golden-file tests per version. |
| `internal/plan` | Add `Coverage string` field; parser validates `xdebug\|pcov\|none` (hard error on anything else, matching v2). Default `none`. |
| `internal/resolve` | New pre-resolution pass applies coverage side-effects to the extension set before regular resolution runs. |
| `internal/compose` | Implement `ini-file` selection (copy chosen base file from bundle to effective `php.ini`; empty file for `none`). Move xdebug ini injection from this package to `internal/compat`. Layer DefaultIniValues + XdebugIniFragment + (pcov ini when applicable) + user `ini-values` into `conf.d/99-buildrush.ini` (last-wins). |
| `cmd/phpup/main.go` | Drop the `ini-file != production` warning. |
| `test/compat/fixtures.yaml` | New fixtures `coverage-pcov` and `ini-file-development`. |
| `docs/compat-matrix.md` | Delete listed allowlist entries; update §5 dispositions for 5.6/5.7/5.8/5.17. |

Unchanged: `internal/oci`, `internal/extract`, `internal/env`, `internal/cache`, `internal/catalog`, `cmd/planner`, `cmd/compat-diff`, and every workflow file other than possibly a fixture-list refresh (which is data-only via §5.6 of the compat-harness design).

### 4.2 The `ini-file` mechanism (chosen option)

Two options were considered:

- **2A — Mirror v2's file-copy mechanic.** Ship `php.ini-production` / `php.ini-development` in the bundle; runtime selects and copies. End-state byte-identical to v2's approach. Chosen.
- **2B — Encode the production/development values in Go.** Rejected: duplicates ~100 upstream keys into our repo; drift goes undetected.

Mechanics under 2A:

1. **Builder step.** After `make install INSTALL_ROOT=/tmp/out`, copy the source-tree `php.ini-production` and `php.ini-development` to `/tmp/out/usr/local/share/php/ini/`. Smoke test asserts both files are present and non-empty.
2. **Compose step.** `internal/compose` reads `$plan.IniFile`, maps it to a filename via `compat.BaseIniFileName`, copies the selected file to `$PHP_PREFIX/lib/php.ini` (or writes empty for `none`). Failing to find the file is a hard error with an explicit message.
3. **Overlay.** `conf.d/99-buildrush.ini` is composed with, in order (last-wins within the file, and the file itself wins because it's the last scan-dir entry):
   - `compat.DefaultIniValues(phpVersion)`
   - `compat.XdebugIniFragment(phpVersion)` if xdebug3 is in the loaded extension set
   - `pcov.enabled=1` if `coverage: pcov` was applied
   - User `ini-values` (already parsed into a map by `internal/plan`)

`ini-file: none` writes an empty `php.ini`; the conf.d overlay still lands. This matches compat-matrix §5.17.

### 4.3 `ini-file` alias handling (compat-matrix §1.1)

v2 accepts five values, fuzzy-mapped by `parseIniFile` (`src/utils.ts` L88-97):

| Input | Maps to |
|---|---|
| `production` (default) | `php.ini-production` |
| `development` | `php.ini-development` |
| `none` | empty file |
| `php.ini-production` | `php.ini-production` |
| `php.ini-development` | `php.ini-development` |
| anything else | `php.ini-production` (fallback) + `::warning::` |

`compat.BaseIniFileName` implements this table exactly; the fallback emits a warning matching v2's silent-fallback behavior (v2 doesn't warn; we warn because silent fallback on unknown input is a compat hazard for our users).

### 4.4 `DefaultIniValues` contents

Per compat-matrix §2.1 and §2.3:

| Key | Value | Condition |
|---|---|---|
| `memory_limit` | `-1` | Always (all 8.x) |
| `date.timezone` | `UTC` | Always (all 8.x) |
| `opcache.enable` | `1` | PHP 8.0–8.9 |
| `opcache.jit_buffer_size` | `256M` | PHP 8.0–8.9 |
| `opcache.jit` | `1235` | PHP 8.0–8.9 |

For pre-8.0 support (out of scope this slice — only 8.4 builds), the function returns just the first two keys. aarch64 gets different `opcache.jit` behavior per compat-matrix §2.4 — the design note there (unfetched upstream content) is the gap, to be resolved in slice #5 (aarch64).

### 4.5 `XdebugIniFragment` contents

Per compat-matrix §2.2:

| Key | Value | Condition |
|---|---|---|
| `xdebug.mode` | `coverage` | xdebug3 loaded AND PHP version matches `7.[2-4]\|8.[0-9]` |

We currently also write `xdebug.start_with_request=default` — that is a deviation we introduced, not a v2 behavior. This slice removes it.

### 4.6 `coverage` side-effect resolution

`internal/plan.FromEnv` parses `INPUT_COVERAGE` into `Plan.Coverage`; the empty default is normalized to `none`. Invalid values fail with `::error::` and a clear message matching v2's phrasing.

`internal/resolve` gains a `ApplyCoverage(plan, extensionSet) extensionSet` step that runs before normal resolution. The step is a pure function over the plan's `Coverage` and `Extensions` inputs:

```go
switch plan.Coverage {
case "xdebug":
    extensions = addIfMissing(extensions, "xdebug")
    extensions = excludeIfPresent(extensions, "pcov")
    // XdebugIniFragment becomes active via compose's xdebug3-detection
case "pcov":
    extensions = addIfMissing(extensions, "pcov")
    extensions = excludeIfPresent(extensions, "xdebug")
    plan.ExtraIni["pcov.enabled"] = "1"
case "none":
    extensions = excludeIfPresent(extensions, "xdebug")
    extensions = excludeIfPresent(extensions, "pcov")
}
```

`ExtraIni` is a new internal map populated by coverage handling and merged into the conf.d overlay after DefaultIniValues. User `ini-values` still beats it.

Order vs extension special syntax (`:ext`, `none` reset): coverage application runs after the extension-list parser has resolved `none` reset and `:ext` exclusions into the final set-to-enable. So `extensions: none, coverage: xdebug` yields `[xdebug]` (not empty). `extensions: :xdebug, coverage: xdebug` yields `[]` — the explicit exclusion wins, matching v2's `disable_extension` running last.

### 4.7 Error handling & warnings

- `coverage` not in `{xdebug, pcov, none, ""}` → `::error::` + exit 1 (matches v2).
- `ini-file` not in the five v2 aliases → `::warning::` + fall back to `production`. Text asserted by test.
- Missing `share/php/ini/php.ini-*` in the bundle → hard error with explicit path. Should never trigger post-rebuild; the smoke test guards it.
- `fail-fast: true` is irrelevant for this slice's new error paths — they are all hard errors or recoverable warnings, not the soft-fallback category.

## 5. Testing

### 5.1 Unit tests

- `internal/compat`:
  - Golden-file assertions on `DefaultIniValues` per version (only 8.4 in scope; other versions land in slice #3).
  - Golden-file assertion on `XdebugIniFragment("8.4")`.
  - `BaseIniFileName` table tests covering all five v2 aliases + one invalid case.
  - Warning-text assertion for the invalid-`ini-file` fallback.
- `internal/plan`:
  - `Coverage` parsing — valid values, invalid values (hard error), default normalization (empty → `none`).
- `internal/resolve`:
  - Table-driven tests for each coverage × starting-extension-set combination:
    - `coverage: xdebug` × (empty, `[redis]`, `[xdebug]`, `[pcov]`, `[xdebug, pcov]`)
    - same for `pcov` and `none`
  - Interaction with `:ext` exclusions and `none` reset (per §4.6 final paragraph).
- `internal/compose`:
  - `ini-file: production` copies `php.ini-production` to effective `php.ini`.
  - `ini-file: development` copies `php.ini-development`.
  - `ini-file: none` writes empty file; overlay still applies.
  - Precedence within conf.d overlay: user `ini-values` beats compat defaults beats nothing.
  - `coverage: pcov` produces `pcov.enabled=1` in the overlay.
  - Bundle-missing-base-file case fails with the expected error.

### 5.2 Smoke

`test/smoke/run.sh` gains an assertion:

```bash
test -s "$BUNDLE_ROOT/usr/local/share/php/ini/php.ini-production"
test -s "$BUNDLE_ROOT/usr/local/share/php/ini/php.ini-development"
```

Runs as part of every bundle's smoke.

### 5.3 Compat harness

Two fixtures added to `test/compat/fixtures.yaml`:

```yaml
- name: coverage-pcov
  php-version: "8.4"
  extensions: ""
  ini-values: ""
  coverage: "pcov"
- name: ini-file-development
  php-version: "8.4"
  extensions: ""
  ini-values: ""
  coverage: "none"
  ini-file: "development"
```

Both must go green with no allowlist entries on the paths they cover.

`docs/compat-matrix.md` allowlist block shrinks by deletion of:

- `ini.display_errors`, `ini.log_errors`, `ini.short_open_tag`, `ini.error_reporting` (fixed by `php.ini-production` copy)
- `ini.opcache.enable`, `ini.opcache.jit`, `ini.opcache.jit_buffer_size` (fixed by DefaultIniValues)
- `ini.xdebug.mode` (multi-ext) and `ini.xdebug.start_with_request` (multi-ext) (fixed by XdebugIniFragment)

Kept:

- `env_delta`, `extensions`, `path_additions` (Phase 3 / later slice scope).
- `ini.opcache.enable_cli`, `ini.opcache.memory_consumption`, `ini.opcache.revalidate_freq`, `ini.opcache.validate_timestamps` — kept until the post-implementation harness run confirms they match automatically via `php.ini-production`. If they do, delete in the same PR; if they don't, the PR blocks and the gap is diagnosed.

### 5.4 Coverage target

Per `CLAUDE.md`: 80% per-package. `internal/compat`, `internal/plan`, `internal/resolve`, `internal/compose` all have existing tests; additions must hold the line or improve.

## 6. Release

- Conventional commits (`feat(compat): …`, `feat(compose): …`, `fix(builders/linux): …`, etc.).
- release-please auto-cuts `v0.2.0-alpha.2` on merge.
- Bundle digest for the 8.4 core changes (added `share/php/ini/` content). PECL extension bundles do not change ABI and do not need rebuild; the existing lockfile entries remain valid.
- `README.md` "Compatibility with shivammathur/setup-php@v2" section (added in slice #1) is updated to reflect the closed gaps.
- `docs/compat-matrix.md` §5 disposition column updated: 5.6/5.7/5.8/5.17 move to "implemented (slice #2)".

## 7. Open questions

- **aarch64 opcache.jit defaults.** Compat-matrix §2.4 flags an unfetched gap in `jit_aarch64.ini`. Not in scope here (aarch64 is slice #5), but `DefaultIniValues` should be structured so the aarch64 variant is a per-arch branch rather than a rewrite.
- **`ini-file: none` for non-default fixtures.** This slice adds one fixture for `ini-file: development`. A `coverage-none` fixture plus an `ini-file-none` fixture could be added if reviewers want broader harness coverage; deferred to avoid fixture-count creep in one slice.
- **`pcov.enabled=1` vs v2's pcov ini mechanism.** v2 adds `pcov.enabled=1` via `add_pcov` (compat-matrix §5.8 / `src/coverage.ts` L61-96). We set the same key via our ExtraIni map. If v2 sets additional pcov keys (confirmed absent in the audit) this deviates; the harness catches it.

## 8. References

- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` — slice #1; §8 lists this closeout as implicit debt.
- `docs/superpowers/specs/2026-04-20-compat-harness-design.md` — slice #2 of the umbrella (the harness); this doc is slice #3 in merge order but "compat closeout" in intent.
- `docs/compat-matrix.md` — §2 (default ini values), §5.6/5.7/5.8 (coverage side-effects), §5.17 (`ini-file: none`), §1.1 note (`ini-file` aliases), deviations allowlist.
- `CLAUDE.md` — quality gates (`make check`), commit style, coverage target.
