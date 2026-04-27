# Design: Emit stock PHP ini defaults to match shivammathur/setup-php@v2

**Issue:** [#79](https://github.com/buildrush/setup-php/issues/79)
**Date:** 2026-04-27
**Status:** Approved (brainstorming)

## Context

PR #77 wired a PR-time compat-diff gate against a pinned shivammathur/setup-php@v2
baseline. The canonical cell (`noble/x86_64/8.4` in `ci.yml::pipeline`) goes red
because `phpup install` returns empty for ~12 ini keys per fixture that v2
emits. These are pre-existing gaps in our compose layer, not regressions
introduced by #77.

The 12 missing keys span three sources:

| Source | Keys |
| --- | --- |
| Stock upstream `php.ini-production` (v2 ships verbatim) | `expose_php`, `log_errors`, `max_input_time`, `session.save_handler`, `session.gc_maxlifetime` |
| Stock upstream `php.ini-production` opcache lines + v2's `src/configs/ini/jit.ini` | `opcache.enable`, `opcache.enable_cli`, `opcache.memory_consumption`, `opcache.revalidate_freq`, `opcache.validate_timestamps`, `opcache.jit`, `opcache.jit_buffer_size` |
| v2's `src/configs/ini/xdebug.ini` (applied under `coverage: xdebug`) | `xdebug.mode`, `xdebug.start_with_request` |

Note that the issue grouped the four "stock opcache" keys under v2's `jit.ini`,
but the audit at `docs/compat-matrix.md` §2.3 (pinned SHA `accd6127…`) confirms
`jit.ini` ships only `opcache.enable`, `opcache.jit`, `opcache.jit_buffer_size`.
The other four (`enable_cli`, `memory_consumption`, `revalidate_freq`,
`validate_timestamps`) originate from upstream `php.ini-production`. We ship
them together because their applicability gate is identical (PHP 8.x and opcache
loaded), and our pipeline overlays one merged map per run.

The runtime already has the right composition primitives — `compose.SelectBaseIniFile`,
`compose.MergeCompatLayers`, `compose.WriteIniValuesWithDefaults`, and three
existing compat fragments (`DefaultIniValues`, `XdebugIniFragment`,
`BaseIniFileName`). The fix is additive: extend the compat-data layer and add
two more layers to the merge pipeline.

## Goals

1. Canonical cell `noble/x86_64/8.4` in `ci.yml::pipeline` goes green without
   any new `kind: ignore` allowlist suppression in `docs/compat-matrix.md`.
2. The weekly `compat-golden-refresh.yml` run continues to show zero drift.
3. The 12 keys' provenance is documented in `docs/compat-matrix.md` so the
   audit chain (upstream → compat-matrix → `internal/compat` constant) stays
   1:1.

## Non-goals

- Investigating or fixing whether `PHPRC=<core>/usr/local/lib/php.ini` actually
  loads the bundle's stock `php.ini-production`. The encode-in-Go approach
  bypasses that question; if PHPRC is broken, file a separate issue.
- Aligning non-canonical cells (other PHP minors, Ubuntu jammy, aarch64) — the
  gate currently only fires on the canonical cell. The fragments are written
  arch- and version-aware, so other cells benefit automatically once the gate
  is extended to them.
- Replicating divergent `ini-file: development` or `ini-file: none` semantics.
  The new defaults apply unconditionally regardless of `ini-file:` input,
  matching the existing precedent for `date.timezone`/`memory_limit`.

## Approach

Encode the 12 missing values in `internal/compat`, exposed via two new
functions and a one-key extension of an existing function. Apply them through
the existing `compose.MergeCompatLayers` pipeline by adding two new layers.
Do not rely on the bundle's stock `php.ini-production` being loaded by PHP at
runtime — overlay everything via `99-user.ini`, which loads after `php.ini`
regardless of how the SAPI resolves its base config.

This is the encode-everything-in-Go option from the brainstorming
discussion. It was preferred over root-causing PHPRC behavior because:

- The compose pipeline and the `compat` data layer already exist; we only grow
  data, no new infrastructure.
- Each key becomes unit-testable in isolation against a golden file, mirroring
  the existing `default_ini_values.golden` pattern.
- The audit chain (compat-matrix.md → `internal/compat` constant → unit-test
  golden → cell-test compat-diff) is uniform per key.
- It is independent of bundle internals: a future bundle change can't silently
  drift the emitted ini surface.

## Detailed design

### 1. `internal/compat/compat.go`

Three changes:

**a) New `StockIniDefaults()`** — keys v2 emits unconditionally on every Linux
run via stock `php.ini-production`:

```go
// StockIniDefaults returns the ini key/value pairs that shivammathur/setup-php@v2
// applies to every Linux run via the stock upstream php.ini-production it
// ships. Always-on, no version or arch gate.
//
// Codifies values v2 actually emits (per the canonical-cell golden), not
// necessarily the literal contents of php.ini-production. For example,
// max_input_time=-1 reflects PHP's CLI-SAPI compiled-in default, which
// overrides php.ini-production's "60" because max_input_time is PHP_INI_PERDIR
// and only takes effect under FPM/CGI.
//
// Data sources: docs/compat-matrix.md §2.1 extended; golden files
// testdata/stock_ini_defaults.golden.
func StockIniDefaults() map[string]string {
    return map[string]string{
        "expose_php":             "1",
        "log_errors":             "1",
        "max_input_time":         "-1",
        "session.save_handler":   "files",
        "session.gc_maxlifetime": "1440",
    }
}
```

**b) New `OpcacheIniFragment(phpVersion, arch string) map[string]string`** —
combines v2's `jit.ini` (3 keys) with the stock `php.ini-production` opcache
lines (4 keys). Returns `nil` for non-8.x versions.

```go
func OpcacheIniFragment(phpVersion, arch string) map[string]string {
    if !isPHP8x(minorOf(phpVersion)) {
        return nil
    }
    out := map[string]string{
        "opcache.enable":              "1",
        "opcache.enable_cli":          "0",
        "opcache.memory_consumption":  "128",
        "opcache.revalidate_freq":     "2",
        "opcache.validate_timestamps": "1",
        "opcache.jit":                 "1235",
    }
    if arch == "aarch64" {
        out["opcache.jit_buffer_size"] = "128M"
    } else {
        out["opcache.jit_buffer_size"] = "256M"
    }
    return out
}
```

**c) `XdebugIniFragment` grows by one key** — `xdebug.start_with_request=default`.
Same gate as today (`xdebug3Supported(minor)`), same caller-side condition
(`coverage: xdebug` drove the install).

**d) `DefaultIniValues` shrinks** — removes the three opcache keys it
currently writes (now in `OpcacheIniFragment`). Keeps `date.timezone=UTC` and
`memory_limit=-1` (compat-matrix §2.1 base, applied unconditionally).

### 2. `internal/compose/compose.go`

Replace `MergeCompatLayers(defaults, xdebugFragment, extra map[string]string)`
with a variadic helper:

```go
// MergeCompatLayers returns a single ini-key map composed of the given layers
// in increasing precedence (last write wins). Any layer may be nil.
// The returned map is always non-nil (may be empty).
func MergeCompatLayers(layers ...map[string]string) map[string]string {
    merged := make(map[string]string)
    for _, layer := range layers {
        maps.Copy(merged, layer)
    }
    return merged
}
```

Variadic was chosen over growing a fixed-arity signature each time we add a
fragment. Three layers today, five tomorrow, possibly more.

### 3. `cmd/phpup/main.go` — wiring

Replace the current 3-layer call at `main.go:407`:

```go
layered := compose.MergeCompatLayers(
    compat.DefaultIniValues(p.PHPVersion, p.Arch),
    xdebugFrag,
    p.ExtraIni,
)
```

with a 5-layer composition:

```go
var opcacheFrag map[string]string
if slices.Contains(bundled, "opcache") && !opcacheExcluded {
    opcacheFrag = compat.OpcacheIniFragment(p.PHPVersion, p.Arch)
}

layered := compose.MergeCompatLayers(
    compat.StockIniDefaults(),                         // always
    compat.DefaultIniValues(p.PHPVersion, p.Arch),     // always
    opcacheFrag,                                       // only if opcache loaded
    xdebugFrag,                                        // only if coverage: xdebug
    p.ExtraIni,                                        // user-configured
)
```

The `opcacheExcluded` variable already exists at `main.go:374`. The opcache
gate reuses it for symmetry with the auto-load logic at `main.go:376`. If
opcache isn't loaded, the keys would be no-ops (PHP ignores ini for unloaded
extensions) but we keep the on-disk fragment clean — same precedent as the
"deliberate divergence from v2" comment on `XdebugIniFragment`.

### 4. Tests (TDD)

**`internal/compat/compat_test.go`** — write before implementation:

- `TestStockIniDefaults` — exact map equality against new
  `testdata/stock_ini_defaults.golden`. No version/arch parameters.
- `TestOpcacheIniFragment` — table test:
  - PHP 7.4: returns `nil`.
  - PHP 8.0–8.5 × {x86_64, aarch64}: equality against
    `testdata/opcache_ini_fragment_x86_64.golden` and
    `testdata/opcache_ini_fragment_aarch64.golden`.
- `TestXdebugIniFragment` — extend existing test (or add) to assert both
  `xdebug.mode=coverage` AND `xdebug.start_with_request=default` for
  xdebug3-supported versions; `nil` for unsupported.
- `TestDefaultIniValues` — adjust existing assertions to reflect the removal
  of opcache keys. Update or delete `testdata/default_ini_values.golden` to
  match the trimmed set.

**`internal/compose/compose_test.go`** — extend `TestWriteIniValuesWithDefaults`
(or add `TestMergeCompatLayers`) to verify:

- Variadic call with 0 layers returns an empty (non-nil) map.
- Last-write-wins ordering across 5 layers.
- `nil` layers are skipped without panic.

**`cmd/phpup/main_test.go` (if such tests exist)** — verify the conditional
gates: opcache fragment present iff opcache is in the loaded set; xdebug
fragment present iff `Coverage == CoverageXdebug`. If `cmd/phpup` lacks unit
coverage of the wiring, rely on the `noble/x86_64/8.4` ci-cell as the
integration verification.

### 5. Documentation — `docs/compat-matrix.md`

- **§2.1** — extend the table with the 5 stock keys. Add a paragraph noting
  `max_input_time=-1` is the CLI-SAPI value (overriding php.ini-production's
  literal `60` because `max_input_time` is `PHP_INI_PERDIR` and FPM/CGI-scoped).
  Cite upstream `php.ini-production` at the PHP-8.4 SHA used by Ondrej PPA at
  the time of audit.
- **§2.3** — extend the table with the 4 additional opcache keys. Note that
  while v2's `jit.ini` only ships 3 keys, our `OpcacheIniFragment` includes 4
  more from stock `php.ini-production` because the load condition (PHP 8.x +
  opcache loaded) is identical, so we ship them together.
- **§2.2** — add `xdebug.start_with_request` row with value `default` and the
  same condition as `xdebug.mode`.
- No `kind: ignore` allowlist entries are added or removed; this PR's job is
  to close the gap, not to mask it. The acceptance criterion is zero
  suppression.

### 6. Quick PHPRC sanity check (orthogonal)

During `make ci-cell OS=noble ARCH=x86_64 PHP=8.4` runs in development, also
print `php --ini` to confirm whether `<core>/usr/local/lib/php.ini` is being
loaded as the base config (i.e. whether `SelectBaseIniFile` is currently
load-bearing or dead code at runtime). Findings:

- If PHPRC is loading the file: the bundled `php.ini-production` is doing its
  job and our overlay still wins via `99-user.ini` (loads later in scan-dir
  order). No further action.
- If PHPRC is not loading the file: file a separate issue. Don't fix in this
  PR — the encode-in-Go approach makes the behavior independent of PHPRC.

This is a probe, not a deliverable.

## Error handling

No new error paths. Existing functions (`SelectBaseIniFile`, `WriteIniValuesWithDefaults`,
`MergeCompatLayers`) already handle empty/nil inputs and disk errors. The
variadic refactor of `MergeCompatLayers` preserves nil-safety per layer.

## Migration / compatibility

- **Existing callers of `MergeCompatLayers`**: only `cmd/phpup/main.go`
  (verified by grep). The variadic signature is source-compatible at the call
  site if all positional layers are passed — no breakage.
- **Goldens for `DefaultIniValues`**: shrink to remove the 3 opcache keys.
  Test data file regenerated as part of this PR.
- **No bundle changes**: the bundle's `php.ini-production` is no longer
  load-bearing for the 12 keys, but it remains shipped (build-php.sh:148) and
  copied (`SelectBaseIniFile`). Behavior unchanged for any consumer of
  `<core>/usr/local/share/php/ini/php.ini-production`.

## Verification

1. `make check` passes.
2. Unit tests for `internal/compat` and `internal/compose` pass.
3. `make ci-cell OS=noble ARCH=x86_64 PHP=8.4` passes including the
   compat-diff gate.
4. The compat-report sticky comment on the resulting PR is empty (or shows
   "cleared") for the canonical cell.
5. Weekly `compat-golden-refresh.yml` next run shows zero drift.

## Open questions

None at this time. Move to implementation planning.
