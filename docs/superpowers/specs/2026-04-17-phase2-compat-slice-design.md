# Phase 2 — Compat-First Slice: Drop-In Parity with `shivammathur/setup-php@v2`

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-17
**Supersedes:** Nothing (first slice of Phase 2 per the phased implementation design)
**Target release:** `v0.2.0-alpha.1`

## 1. Summary

Phase 2 of `buildrush/setup-php` is an umbrella for Linux matrix expansion (PHP 8.1–8.5, top-50 extensions, x86_64 + aarch64, ubuntu-22.04 + ubuntu-24.04). This document specifies **sub-project #1 of Phase 2**: a compat-first slice that lands drop-in compatibility with `shivammathur/setup-php@v2` for the single PHP version and architecture Phase 1 already builds (PHP 8.4 NTS, linux/x86_64, ubuntu-24.04), with the catalog restructured to encode per-version compat intent for 8.1–8.5 (without building them yet).

The slice ships **before** matrix expansion so that users migrating from `shivammathur/setup-php@v2` get drop-in behavior as soon as they move to the versions/arches we cover, and so that the matrix-expansion slices that follow build against a compat-correct baseline rather than rebuilding to fix compat drift later.

## 2. Goals & non-goals

### Goals
- Every input declared by `shivammathur/setup-php@v2` is declared in our `action.yml` with matching names, types, defaults, and required/optional flags.
- Every input we act on behaves the same way theirs does, including special syntaxes (extension exclusion, reset, etc.) and search orders.
- Defaults shivammathur applies on CI runners (ini-values, coverage, etc.) are applied by us unless the user overrides.
- The PHP 8.4 core bundle is rebuilt so its compile-time bundled extension set matches the set `shivammathur/setup-php@v2` exposes on Linux (sourced from the Ondrej PPA build).
- Inputs we cannot implement given our architecture (e.g. `update`) are accepted for parseability and emit a clear warning rather than failing.
- The `tools` input continues to parse-and-warn in this slice; real tools support is Phase 3.
- Compat decisions and deviations are documented in `docs/compat-matrix.md`.

### Non-goals
- Building additional PHP versions (8.1/8.2/8.3/8.5). They are encoded in the catalog but not built.
- aarch64. Stays x86_64 only.
- ubuntu-22.04. Stays ubuntu-24.04 only.
- Top-50 extension expansion. Extension set stays at the current 4 PECL entries plus the rebuilt 8.4 built-ins.
- Tool installation (composer, phpunit, phpstan, etc.) — Phase 3.
- Tiered fallbacks — Phase 3.
- Automated compat-harness workflow that diffs our behavior against `shivammathur/setup-php@v2` at runtime. This is a planned follow-up slice under the Phase 2 umbrella (see §8).
- macOS/Windows — Phases 5/6.

## 3. Source of truth

Compat is audited against a pinned reference so the work is reproducible and drift is attributable:
- `shivammathur/setup-php` — pinned to a specific commit SHA, captured in `docs/compat-matrix.md`. The audit reads `action.yml`, `README.md`, and `src/` (TypeScript sources that implement the scripts).
- Ondrej PPA (`ppa:ondrej/php`) — pinned to a specific snapshot date, captured in `docs/compat-matrix.md`. Used to determine bundled-extension sets per PHP version.

When the pinned references are bumped, changes to compat data require a deliberate PR whose description names the old and new SHA/snapshot.

## 4. Compat layers in scope

| Layer | Description | Scope in this slice |
|-------|-------------|---------------------|
| L1 | Input shape in `action.yml` (names, types, defaults) | Full — every v2 input declared |
| L2 | Semantic compat on inputs we implement | Full — `php-version`, `php-version-file`, `extensions`, `ini-values`, `coverage` |
| L3 | Default ini-values + per-version built-in extension sets | Full — defaults applied at runtime; 8.4 rebuilt; catalog encodes 8.1–8.5 |
| L4 | Output compat (`php-version` string format) | Full |
| L5 | Behavioral quirks (Composer auto-install, env var exports, etc.) | Audit fully; implement cheap quirks; remaining documented with follow-up tracking |

## 5. Architecture

### 5.1 Component layout

```
action.yml                         ← declare all v2 inputs (add phpts, update, fail-fast, …)
src/index.js                       ← pass new INPUT_* env vars through to phpup (no logic)
cmd/phpup/main.go                  ← emit no-op warnings early; set php-version output (X.Y.Z)
internal/plan/                     ← parse new inputs; full extension-syntax compat
internal/compat/          (new)    ← single source of truth for "what v2 does":
                                      - DefaultIniValues(phpVersion) map[string]string
                                      - BundledExtensions(phpVersion) []string
                                      - UnimplementedInputWarning(name, value) string
internal/resolve/                  ← phpts/fail-fast semantics
internal/compose/                  ← merge compat default ini-values; honor :ext exclusions
catalog/php.yaml                   ← restructure to versions: {8.1..8.5: {...}}; only 8.4 has sources
builders/linux/build-php.sh        ← 8.4 rebuild with compat-matching configure flags
docs/compat-matrix.md     (new)    ← per-input table, quirk catalog, pinned references
test/smoke/compat.sh      (new)    ← end-to-end compat scenario against rebuilt 8.4
```

### 5.2 Data flow

```
action.yml inputs (all v2 keys declared)
    → src/index.js (env pass-through)
    → cmd/phpup/main.go
        → emit warnings for unimplementable inputs
    → internal/plan
        → parse new fields (phpts, update, fail-fast, …)
        → extension-syntax parser: :ext, none, mixed lists
    → internal/resolve
        → apply phpts semantics (zts → warn+fallback, or fail under fail-fast: true)
        → apply fail-fast: true (promote warnings to errors)
    → internal/extract (unchanged)
    → internal/compose
        → compat.DefaultIniValues merged in (user-supplied overrides)
        → :ext exclusions produce disable-*.ini fragments
    → internal/env (export, unchanged)
    → output: php-version=X.Y.Z (full triple)
```

The `internal/compat` package is the **one place** where "what shivammathur does" lives. Other packages call into it. Compat drift → one-file change.

### 5.3 `action.yml` input additions

Inputs added in this slice (pending audit — this is the baseline from the vision doc):

| Input | Default | Behavior in this slice |
|-------|---------|------------------------|
| `phpts` | `nts` | `nts` honored; `zts` → warn + fallback to NTS (error under `fail-fast: true`) |
| `update` | `false` | Accepted, no-op, warn if `true` |
| `fail-fast` | `false` | `true` promotes soft-fallback warnings to errors |

Inputs already declared but audited for default/description parity: `php-version`, `extensions`, `ini-values`, `coverage`, `tools`, `php-version-file`.

Any additional inputs discovered during the audit are added with the same pattern: declared with matching default, implemented semantically if cheap, otherwise no-op + warn.

### 5.4 Extension special-syntax parser

`internal/plan` extends its `extensions` parser to match shivammathur v2's accepted syntax. The authoritative list is extracted from their source during the audit; the expected cases are:

- `redis, xdebug` — list of extensions to enable (existing behavior).
- `:opcache` — exclude a built-in extension. Runtime generates a `conf.d/disable-opcache.ini` or equivalent so the composed PHP does not load it.
- `none` — reset: start from an empty extension set. Any extensions listed *after* `none` are the complete set.
- `none, redis, xdebug` — start empty, then add `redis` and `xdebug` (no built-ins loaded).
- Case/whitespace handling matches theirs (trim, lower-case names).

If the audit uncovers additional syntax (version pinning via `ext@version`, for example), each case either lands here or is documented as deferred in `compat-matrix.md`.

### 5.5 `internal/compat` package

Single file, exports three pure functions and a small set of package-level maps as data:

```go
// DefaultIniValues returns the ini key/value pairs that
// shivammathur/setup-php@v2 sets on Linux CI runners by default.
// User-supplied ini-values take precedence over these.
func DefaultIniValues(phpVersion string) map[string]string

// BundledExtensions returns the set of extensions compiled-in to the
// shivammathur/setup-php@v2 Linux build for a given PHP version
// (i.e. what `php -m` reports after `setup-php` runs with no
// `extensions:` input on Ondrej PPA's build for that version).
// Used by the compose layer to determine what :ext exclusions
// need to act on and what `none` needs to "reset from".
func BundledExtensions(phpVersion string) []string

// UnimplementedInputWarning returns the canonical warning line
// (including the ::warning:: Actions prefix) emitted when a
// no-op input is set to a non-default value.
func UnimplementedInputWarning(inputName, value string) string
```

The per-version data lives in the same package as frozen Go maps. A companion `_test.go` file asserts the maps against golden files, so a change to compat data fails the test unless the golden is updated deliberately (forcing a reviewer to acknowledge the compat shift).

### 5.6 Catalog restructure

`catalog/php.yaml` moves from a single-version flat file to:

```yaml
# catalog/php.yaml
versions:
  "8.1":
    bundled_extensions: [mbstring, curl, intl, zip, …]
    # no sources: block ⇒ builder pipeline skips
  "8.2":
    bundled_extensions: [...]
  "8.3":
    bundled_extensions: [...]
  "8.4":
    bundled_extensions: [...]
    sources:
      version: 8.4.5
      url: https://www.php.net/distributions/php-8.4.5.tar.xz
      sha256: ...
    abi_matrix:
      - {os: linux, arch: x86_64, ts: nts}
  "8.5":
    bundled_extensions: [...]
```

- The planner (`cmd/planner/`) already operates on the catalog; its extension/PHP-core build matrix skips entries without `sources:`.
- The extraction/compose runtime (`cmd/phpup/`) only consults `bundled_extensions` for the version actually in the lockfile, so catalog entries for unbuilt versions are inert at runtime.
- This is a data-only change for 8.1/8.2/8.3/8.5; only 8.4 has a builder effect.

### 5.7 PHP 8.4 rebuild

`builders/linux/build-php.sh` is updated to pass the `./configure` flag set that produces a build with the same bundled extensions as `shivammathur/setup-php@v2` exposes on linux/x86_64. The diff between our current 8.4 build and the target is enumerated in `docs/compat-matrix.md` before implementation.

The existing 4 PECL extension bundles (redis, xdebug, pcov, apcu) are rebuilt against the new 8.4 core if and only if the audit shows an ABI-affecting change. In most cases separately-loadable `.so` files remain compatible; the audit step confirms or denies.

## 6. Error handling & warnings

- **Unimplementable inputs** (`update`, possibly others from audit): accepted, no-op, emit `::warning::` line via `compat.UnimplementedInputWarning`. Warning text is asserted by tests so it stays stable across releases.
- **Soft-fallback decisions** (ZTS requested but not built, extension unresolved, etc.): default to `::warning::` + fallback. Under `fail-fast: true`, each becomes a hard error with a matching `::error::` line.
- **Parse errors on recognized inputs**: fail with matching wording where shivammathur fails; accept-and-ignore where they do.
- **Compat-data test failure**: forces a deliberate golden-file update, which means a reviewer sees the shift in the PR diff.

## 7. Testing strategy

- **`internal/plan` tests**: full coverage of extension special syntax (exclusion, reset, mixed ordering, whitespace, case), and parsing of new inputs with all accepted values.
- **`internal/compat` tests**: golden-file assertions on `DefaultIniValues` and `BundledExtensions` for each cataloged PHP version, plus exact warning-text assertion.
- **`internal/compose` tests**: default-ini merge precedence (user beats compat), exclusion-fragment generation, interaction with `coverage: none`.
- **`internal/resolve` tests**: `phpts: zts` warn/fallback path; `fail-fast: true` promotes the same scenarios to error.
- **`cmd/phpup` integration**: a test that builds a fake plan with `update: true` + `tools: composer` and asserts both expected warning lines appear on `stderr`.
- **Smoke (`test/smoke/compat.sh`)**: exercises `extensions: :opcache, redis`, `ini-values: memory_limit=256M,date.timezone=UTC`, `coverage: xdebug` end-to-end against the rebuilt 8.4 bundle. Asserts `php -m` includes redis and excludes opcache; `php --ini` shows the expected config path; `php -r 'echo ini_get("memory_limit");'` returns `256M`; xdebug module loaded.
- **Existing tests**: untouched except where the plan/resolve/compose refactors intersect. Coverage target per `CLAUDE.md` (80%/package) must not regress.

## 8. Follow-up slices (Phase 2 umbrella, out of this slice)

Listed here so the overall Phase 2 shape is visible. Each gets its own spec → plan → impl cycle.

1. **Compat harness (V2 verification)** — a CI workflow that runs fixture input matrices through both `shivammathur/setup-php@v2` and `buildrush/setup-php`, diffs `php -v` / `php -m` / `php --ini` / exported env / `$PATH` entries, fails on deviation. Can run once we have ≥2 PHP versions built so it's meaningful.
2. **PHP 8.1–8.3, 8.5 builds on x86_64** — flip on the `sources:` block for each version, exercise the pipeline.
3. **Extension matrix expansion to top-50** — catalog + build.
4. **aarch64** — builder + abi_matrix + runner work.
5. **ubuntu-22.04 runner coverage** — test matrix + any version-specific compat notes.
6. **Phase 2 exit release** — `v0.2.0-alpha` (no `.1`) once (2)–(5) land.

## 9. Release

- Conventional commits throughout.
- release-please cuts `v0.2.0-alpha.1` (or next appropriate alpha increment) when the compat slice merges.
- README gets a "Compatibility with shivammathur/setup-php@v2" section linking to `docs/compat-matrix.md`.
- A tracking issue (or discussion) lists deferred L5 quirks so they are visible to users and contributors.

## 10. Open questions

- **Exact list of v2 inputs** — settled by the audit; any input beyond the baseline (`php-version`, `extensions`, `ini-values`, `coverage`, `tools`, `php-version-file`, `phpts`, `update`, `fail-fast`) lands in `action.yml` with the same pattern.
- **Ondrej PPA bundled-extension snapshot date** — chosen during implementation; pinned in `compat-matrix.md`.
- **Warning phrasing** — drafted during implementation; asserted by tests.
- **L5 quirk list** — produced by the audit; each quirk resolved to "implemented here", "documented deviation", or "follow-up issue #N".

## 11. References

- `docs/product-vision.md` — overall product rationale and compat targets.
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` — phase decomposition; this slice is sub-project #1 of Phase 2 (§4 of that doc).
- `CLAUDE.md` — quality gates (must pass `make check`), commit style, coverage target.
