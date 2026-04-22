# Phase 2 — Cross-OS Ubuntu Support via Hermetic Library Capture

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-22
**Supersedes:** `2026-04-22-phase2-ubuntu-22.04-design.md` (slice D, rolled back — PR #50 closed unmerged)
**Closes:** GitHub issue #51
**Target release:** Next patch alpha. Phase-2 exit tag rename (`v0.2.0-alpha` un-suffixed) tracked separately as a release-please config concern.

## 1. Summary

Replacement for the rolled-back Phase-2 Slice D (`phase2-ubuntu-22.04`). That slice assumed glibc forward-compatibility was sufficient to let a jammy-compiled bundle run on a noble runner; it wasn't — `--enable-intl` links `libicui18n.so.70` (jammy's ICU) which noble doesn't carry, and imagick links against ImageMagick 6 with a jammy-specific SOVERSION that has no noble transitional.

This slice introduces a **general cross-OS mechanism**: each bundle can declare `hermetic_libs` (a list of filename globs) that the builder captures into the bundle itself, and the compiled binary's embedded rpath loads them at runtime regardless of the runner's apt state. The framework is deliberately generic — supporting a new Ubuntu release (25.04, 26.04) or a new distro family is a catalog edit plus an audit-driven rebuild, not a schema change.

Drift set today: ICU (core, 4 `.so`s) and ImageMagick (imagick, 3 `.so`s). The slice enables hermetic capture for both plus the generic framework plus a governance tool (`cmd/hermetic-audit`) that surfaces future drift automatically.

Wall-clock: all 182 bundles rebuild in one pipeline run (~45–60 min ceiling, dominated by grpc). Lockfile diff churns all 182 entries — same scale as slice D would have been.

## 2. Goals & non-goals

### Goals
- `catalog/*` gains an optional `hermetic_libs: []glob` field on each PHP version block and on each extension.
- `catalog/php.yaml` declares ICU globs on every version (8.1–8.5): `libicui18n.so.*`, `libicuuc.so.*`, `libicudata.so.*`, `libicuio.so.*`.
- `catalog/extensions/imagick.yaml` declares `libMagickWand-6.Q16.so.*`, `libMagickCore-6.Q16.so.*`, `libltdl.so.*`, and drops the two magickwand entries from its `runtime_deps.linux`.
- `builders/common/capture-hermetic-libs.sh` (new shared script) runs after `make install`, matches the declared globs against `ldd` output (transitively, glob-whitelisted), copies matched `.so`s into the bundle's hermetic directory, and asserts the target binary's rpath points at that directory.
- `builders/linux/build-php.sh` and `builders/linux/build-ext.sh` add rpath `LDFLAGS` and invoke the capture script.
- `internal/planner.ComputeSpecHash` folds `BUILDER_OS` into the hash (re-introduced from rolled-back slice D). `BUILDER_OS=ubuntu-22.04` pinned in `builders/common/builder-os.env`.
- Bundle `schema_version` bumps from 2 → 3; `internal/version.MinBundleSchema("php-core")` and `…("php-ext")` bump to 3. `meta.json` gains `hermetic_libs: [resolved filenames]` and `builder_os: "ubuntu-22.04"`.
- `cmd/hermetic-audit` (new) verifies a bundle's ELF loads cleanly against a given runner OS via `ldd` in a matched Docker image, failing with a concrete "add this glob" suggestion when drift is detected.
- `.github/workflows/compat-harness.yml` fixture matrix gains `runner_os` (default `ubuntu-24.04`); 5 new `bare-jammy-<NN>` fixtures (x86_64 only) pin `ubuntu-22.04`; a new pre-flight `hermetic-audit` job gates `ours`/`theirs`.
- All 182 bundles rebuild; lockfile auto-refreshes. Jammy-built bundles load on both `ubuntu-22.04` and `ubuntu-24.04` runners (x86_64 + aarch64) for every existing fixture.
- `README.md` documents jammy+noble supported runners; the noble-only caveat is removed.

### Non-goals
- Per-OS bundle keying (issue #51 option `B`). Explicitly rejected — permanent matrix doubling, scales linearly with every new Ubuntu release, trades build cost for a property that hermetic capture delivers for free.
- Static linking of ICU/ImageMagick into PHP/`.so` (option `E`). Explicitly rejected — binary bloat, CVE response requires full PHP rebuild.
- Dropping `--enable-intl` from core (option `D`-variant). Rejected — feature regression.
- Runtime `.deb` fetch from `archive.ubuntu.com` at install time. Rejected — trades build-time bytes for runtime fragility and a new trust root.
- macOS / Windows hermetic-lib capture. The catalog field is shaped so Phase 5 / 6 can declare platform-specific globs, but no darwin/windows work lands here.
- PECL version bumps (#38 redis, #43 igbinary). Separate single-purpose slices.
- `v0.2.0-alpha` un-suffixed exit tag. Release-please config follow-up; orthogonal to the engineering work.
- Phase-7 `security-rebuild.yml` wiring against the new hermetic-lib CVE surface. Phase-7 scope; this slice establishes `hermetic_libs` as the canonical list that `security-rebuild` will later consult.
- Migration for users on prior alpha tags. They stay where they are; `@main` / next alpha is the forward path.

## 3. Source of truth

Same pinning as every Phase-2 slice since #1:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` (no new audit needed — no v2 behavior changes across OSes) |
| Builder OS | `ubuntu-22.04` (jammy — pinned in `builders/common/builder-os.env`) |

### 3.1 Why jammy is the builder pin

Oldest currently-supported LTS in the Phase-2 runner set. glibc 2.35 is the lowest common denominator; a jammy-built binary is glibc-forward-compatible to noble (2.39) and any future LTS. The `.so`s it links against are the drift problem hermetic capture solves; glibc itself is fine.

Future options (e.g. moving the builder to noble once jammy EOLs, or to 26.04 when it ships) require a `BUILDER_OS` file edit and a pipeline re-run — nothing structural.

### 3.2 The drift set today

| Extension / Core | Linked lib | jammy SOVERSION | noble SOVERSION | Transitional on noble? |
|---|---|---|---|---|
| core (`--enable-intl`) | `libicui18n` | `.so.70` | `.so.74` | ✗ no `libicu70` on noble |
| core (`--enable-intl`) | `libicuuc` | `.so.70` | `.so.74` | ✗ |
| core (`--enable-intl`) | `libicudata` | `.so.70` | `.so.74` | ✗ |
| core (`--enable-intl`) | `libicuio` | `.so.70` | `.so.74` | ✗ |
| imagick | `libMagickWand-6.Q16` | `.so.6` | `.so.7` | ✗ different SOVERSION, no back-port |
| imagick | `libMagickCore-6.Q16` | `.so.6` | `.so.7` | ✗ |
| imagick | `libltdl` | `.so.7` | `.so.7` | ✓ (listed defensively — audit may prune) |

Transitional packages carry the `t64` jammy→noble transition for libs that bumped SOVERSION due to 64-bit `time_t`; those resolve automatically and do not need hermetic capture. The audit confirms this per-extension.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/` | `PHPVersionSpec` and `ExtensionSpec` gain optional `hermetic_libs: []string`. `catalog/php.yaml` populates on every version. `catalog/extensions/imagick.yaml` populates + trims `runtime_deps.linux`. |
| `internal/catalog` | Struct field + YAML unmarshal + validation (glob syntax). Round-trip tests. |
| `builders/common/capture-hermetic-libs.sh` | **New.** Shared script invoked by both the core and extension builders. Glob-whitelisted transitive `ldd` walk; copies matches; asserts rpath. |
| `builders/common/builder-os.env` | **New.** One line: `BUILDER_OS=ubuntu-22.04`. Read by planner for hashing; interpolated by workflows for `runs-on`. |
| `builders/linux/build-php.sh` | Adds `-Wl,-rpath,$ORIGIN/../lib/hermetic -Wl,-rpath,$ORIGIN/../lib` to `LDFLAGS`. Invokes capture after `make install`. |
| `builders/linux/build-ext.sh` | Adds `-Wl,-rpath,$ORIGIN/hermetic` to `LDFLAGS`. Invokes capture after `make install`. |
| `internal/planner` | `ComputeSpecHash` signature gains `builderOS string`. `builderHash` continues to hash the builder script set (now including `capture-hermetic-libs.sh`). |
| `internal/bundle` / sidecar code | `meta.json` gains `hermetic_libs: []string` and `builder_os: string`. Writers populate from capture output; readers expose for audit. |
| `internal/version` | `MinBundleSchema("php-core") = 3`, `…("php-ext") = 3`. |
| `cmd/hermetic-audit/` | **New.** Standalone Go tool. Docker-backed `ldd` run inside the target runner's image. Emits machine-readable diff + human-readable fix suggestion. |
| `.github/workflows/build-php-core.yml`, `build-extension.yml` | `runs-on: ${{ inputs.arch == 'aarch64' && 'ubuntu-22.04-arm' \|\| 'ubuntu-22.04' }}`. |
| `.github/workflows/compat-harness.yml` | Fixture matrix gains `runner_os`. `ours`/`theirs` `runs-on` pivots on `matrix.runner_os × matrix.arch`. New pre-flight `hermetic-audit` job gates the pair. |
| `test/compat/fixtures.yaml` | 5 new `bare-jammy-<NN>` x86_64 fixtures; existing 65 default to `runner_os: ubuntu-24.04`. |
| `cmd/phpup/main.go` | No runtime logic change. Runtime catalog map's imagick `RuntimeDeps` mirrors the trimmed catalog. |
| `docs/bundle-schema-changelog.md` | New v3 entry. |
| `README.md` | "Supported runners" — jammy + noble, x86_64 + aarch64. |

Explicitly **unchanged**: `internal/resolve`, `internal/compose`, `internal/oci`, `internal/extract`, `internal/env`, `internal/cache`, `internal/lockfile`, `bundles.lock` key schema.

### 4.2 Catalog schema

Optional field on each PHP version block:

```yaml
# catalog/php.yaml
versions:
  "8.4":
    source: {...}
    abi_matrix: {...}
    hermetic_libs:
      - libicui18n.so.*
      - libicuuc.so.*
      - libicudata.so.*
      - libicuio.so.*
```

Optional field on each extension:

```yaml
# catalog/extensions/imagick.yaml
hermetic_libs:
  - libMagickWand-6.Q16.so.*
  - libMagickCore-6.Q16.so.*
  - libltdl.so.*
runtime_deps:
  linux: []   # magickwand apt packages dropped — libs are hermetic now
```

**Validation** (in `internal/catalog` load):
- Each glob must match `^lib[A-Za-z0-9._+-]+\.so(\.[A-Za-z0-9.*]+)?$`. Reject anything containing `/` (filename-only; no directory components).
- Reject globs that don't contain `*` and don't look like a concrete SOVERSION (authors are nudged toward forward-compat globs).
- Duplicate detection within a single list.

Absence of `hermetic_libs` means "no hermetic capture for this bundle" — backward-compatible with existing catalog entries.

### 4.3 Capture script contract

```
builders/common/capture-hermetic-libs.sh \
  --target /tmp/out/usr/local/bin/php \
  --globs libicui18n.so.*,libicuuc.so.*,libicudata.so.*,libicuio.so.* \
  --output /tmp/out/usr/local/lib/hermetic \
  [--allow-missing-glob]
```

**Semantics:**
1. Run `ldd $TARGET`. Parse `name => /absolute/path` pairs.
2. For each glob, `fnmatch` against each parsed `name`. Collected matches = "first wave."
3. For each file in "first wave," run `ldd` on IT. Any `name` in its output that matches *any* glob joins the capture set. Iterate to fixpoint. Non-matching transitive deps (`libc.so.6`, `libstdc++.so.6`, ...) are explicitly skipped — they come from the runner.
4. Copy resolved files into `$OUTPUT`, preserving SOVERSION. Symlink chains inside `/usr/lib/x86_64-linux-gnu/` are recreated inside `$OUTPUT` using `cp -P` semantics so the linker sees the same names it did at build time.
5. Verify `readelf -d $TARGET` lists `$OUTPUT` via `DT_RUNPATH` / `DT_RPATH`. If not, fail with a message pointing at the builder's `LDFLAGS`.
6. Emit stdout JSON: `{"captured": ["libicui18n.so.70", ...], "skipped_system": ["libc.so.6", ...]}`. Builder pipes this into `meta.json.hermetic_libs`.

**Failure modes** (all non-zero exit, `::error::` line):
- Glob has zero matches in `ldd` output → unless `--allow-missing-glob`, fail. Catches "author added a glob but PHP wasn't configured with the feature that links it."
- Target binary lacks the expected rpath → fail.
- Copy target already exists with different SHA → fail. Catches stale output dirs on local dev.

Default behavior fails on missing glob; `--allow-missing-glob` is opt-in for conditional extensions and is documented in the script header.

### 4.4 Bundle layout and schema v3

PHP core bundle post-capture:

```
usr/local/bin/php                           (rpath: $ORIGIN/../lib/hermetic:$ORIGIN/../lib)
usr/local/bin/php-cgi                       (same)
usr/local/bin/phpize
usr/local/bin/php-config
usr/local/lib/php/extensions/<abi>/*.so
usr/local/lib/hermetic/libicui18n.so.70     # NEW
usr/local/lib/hermetic/libicuuc.so.70       # NEW
usr/local/lib/hermetic/libicudata.so.70     # NEW
usr/local/lib/hermetic/libicuio.so.70       # NEW
usr/local/share/php/ini/php.ini-production
usr/local/share/php/ini/php.ini-development
meta.json
```

Extension bundle (imagick):

```
imagick.so                                  (rpath: $ORIGIN/hermetic)
hermetic/libMagickWand-6.Q16.so.6           # NEW
hermetic/libMagickCore-6.Q16.so.6           # NEW
hermetic/libltdl.so.7                       # NEW
meta.json
```

rpath uses `$ORIGIN` relative semantics so the path resolves regardless of where `compose` extracts the bundle on the runner. No `LD_LIBRARY_PATH` is set at runtime.

`meta.json` (schema v3):

```json
{
  "schema_version": 3,
  "kind": "php-core",
  "hermetic_libs": [
    "libicui18n.so.70",
    "libicuuc.so.70",
    "libicudata.so.70",
    "libicuio.so.70"
  ],
  "builder_os": "ubuntu-22.04",
  ...existing fields...
}
```

`hermetic_libs` is the resolved-filename list (not the globs). `builder_os` surfaces to `phpup doctor` for support.

**Schema bump contract:**
- `internal/version.MinBundleSchema("php-core") = 3`, `…("php-ext") = 3`.
- Runtime rejects any v2 bundle with a hard error pointing at `docs/bundle-schema-changelog.md`.
- Migration window: none. The PR introduces schema v3 and `MinBundleSchema = 3` together. The pipeline run that lands on merge rebuilds all 182 cells at v3 atomically. Users on prior alpha tags retain their old behavior; the forward path is the next alpha.

### 4.5 Planner spec hash and force-rebuild scale

```go
func ComputeSpecHash(cell *MatrixCell, catalogData []byte, builderHash, builderOS string) string {
    h := sha256.New()
    h.Write(catalogData)            // includes hermetic_libs — catalog edits invalidate
    h.Write([]byte(builderHash))    // includes capture-hermetic-libs.sh
    h.Write([]byte(builderOS))      // "ubuntu-22.04" today
    _, _ = fmt.Fprintf(h, "%s:%s:%s:%s:%s:%s",
        cell.Version, cell.Extension, cell.OS, cell.Arch, cell.TS, cell.PHPAbi)
    return fmt.Sprintf("sha256:%x", h.Sum(nil))
}
```

`builderHash` hashes the full builder script set including the new `capture-hermetic-libs.sh`. `builderOS` is read from `builders/common/builder-os.env`.

Every spec_hash changes on merge because (a) catalog grows `hermetic_libs`, (b) `BUILDER_OS` enters the hash, (c) `capture-hermetic-libs.sh` is new in `builderHash`. All 182 cells rebuild in one pipeline run (~45–60 min ceiling, dominated by the 10 grpc cells). The `bundles.lock` diff churns all 182 `digest` and `spec_hash` fields — same scale as slice D would have been.

### 4.6 Governance: `cmd/hermetic-audit` and harness integration

The audit tool is the generalization mechanism — the piece that makes future Ubuntus (and any new runner OS) absorbable without architecture churn.

**Tool:**

```
hermetic-audit \
  --bundle /path/to/extracted/bundle \
  --expected-runner-os ubuntu-24.04 \
  [--format {human|json}]
```

For each ELF in the bundle (core's `php`/`php-cgi`/...; each extension's `.so`; libs inside the hermetic dir itself):

1. Run `ldd $ELF` inside a clean `ubuntu:22.04` / `ubuntu:24.04` Docker image matching `--expected-runner-os`. The tool ships a thin Docker wrapper so the audit runs from any CI host.
2. Parse `ldd` output for `not found` entries. Each such entry = a lib the runner can't provide AND the bundle didn't capture.
3. Cross-reference missing entries against the bundle's `meta.json.hermetic_libs`. An entry that's missing AND matches no declared glob → audit failure with a specific "add `<glob>` to `hermetic_libs` of `<bundle>`" suggestion. An entry that IS in `meta.json.hermetic_libs` but still shows missing → capture-script bug (internal error).
4. Confirm captured libs resolve *inside* the bundle via rpath — that the rpath wiring actually works on the target runner.

Exit codes: `0` clean, `1` drift found (with diff), `2` tool/environment error.

**Harness integration:**
- `compat-harness.yml`'s fixture matrix gains `runner_os: [ubuntu-22.04, ubuntu-24.04]`. Default `ubuntu-24.04`; 5 new `bare-jammy-<NN>` x86_64 fixtures pin `ubuntu-22.04`.
- A new pre-flight job `hermetic-audit` runs before `ours`/`theirs`: for each `(bundle, runner_os)` pair exercised by the fixture matrix, download the bundle, run `hermetic-audit`. Failure blocks `ours`/`theirs` for that fixture — fast failure with a concrete fix.
- arm64 covered identically (`ubuntu-22.04-arm`, `ubuntu-24.04-arm`); runner_os axis is orthogonal to arch.

**How the framework absorbs a new OS:**
1. Add the runner label to the harness fixture matrix.
2. Run CI. `hermetic-audit` either passes on every bundle (nothing drifted) or reports exactly which globs to add to which catalog entries.
3. Add the globs. Re-run. Green.

No builder changes. No schema changes. No `phpup` code changes.

### 4.7 Error handling

- Catalog with invalid `hermetic_libs` glob syntax → planner fails with a line/column pointer at the offending glob.
- Capture script's failure modes per §4.3.
- Audit tool failure per §4.6.
- Runtime: schema-v2 bundle with `MinBundleSchema = 3` → hard error with `docs/bundle-schema-changelog.md` pointer. Not silent fallback — users need to know their installed alpha is incompatible with the bundle they fetched.
- Runtime: no new file-existence check for hermetic libs. Audit + smoke catch missing captures before the bundle ships; belt-and-braces runtime checks are explicitly out of scope here to honor §4.1's "no runtime logic change" invariant.

## 5. Testing

### 5.1 Unit tests

**`internal/catalog`:**
- Round-trip of `hermetic_libs` (YAML → struct → YAML stable).
- Validation: rejects globs with `/`, accepts the four ICU patterns, accepts the three imagick patterns.
- Rejects globs that are neither wildcards nor concrete SOVERSIONs (per §4.2 rule).
- Duplicate glob rejection; empty-list acceptance.

**`internal/planner`:**
- `TestComputeSpecHash_BuilderOSIsLoadBearing` (two cells, same catalog/builder hash, different `BUILDER_OS` → different hashes) — slice-D test reinstated.
- `TestComputeSpecHash_HermeticLibsIsLoadBearing` (catalog with vs without `hermetic_libs` → different hashes via the `catalogData` branch).

Coverage for "capture-script bytes invalidate cells" lives one layer up in `cmd/planner` (where `builderHash` is computed from the builder script set) and the bats tests in §5.2. `internal/planner` only sees `builderHash` as an opaque string.

**`internal/bundle` / `internal/version`:**
- `MinBundleSchema("php-core") == 3`, `…("php-ext") == 3`.
- Schema-2 sidecar rejected with a message containing `docs/bundle-schema-changelog.md`.
- `meta.json` writer emits `hermetic_libs` and `builder_os`; reader round-trips them.

**`cmd/hermetic-audit`:**
- Table-driven against fixture bundles:
  - Clean bundle on supported runner → pass.
  - Missing glob on runner that needs it → fail with specific "add glob" diff.
  - Captured lib but rpath broken → fail with "rpath missing" message.
  - Bundle without `hermetic_libs` on a runner that needs one → fail pointing at which glob to add.

### 5.2 Builder-script tests

`test/builders/capture-hermetic-libs.bats` (new):
- Empty glob list → no-op, exit 0.
- Single-match glob → captures, asserts output dir contents.
- Transitive walk → captures only glob-matching transitive deps; skips non-matching system libs.
- Missing glob → fail.
- Missing rpath on target → fail.
- `--allow-missing-glob` → passes with empty match.

Uses small fixture ELF binaries (preferably a `busybox`-style stub plus synthesized `.so`s) to avoid requiring a full PHP toolchain for the capture unit tests.

### 5.3 Smoke tests (per bundle, in `test/smoke/run.sh`)

- PHP core: `test -d "$BUNDLE/usr/local/lib/hermetic" && test -s "$BUNDLE/usr/local/lib/hermetic/libicui18n.so."*`.
- Extension declaring `hermetic_libs`: `test -d "$BUNDLE/hermetic"` + concrete file existence per catalog list.
- Every ELF: `readelf -d $ELF | grep -E 'R(UN)?PATH' | grep hermetic` passes.
- Functional: `php -r 'echo INTL_ICU_VERSION;'` on a bare-jammy fixture resolves — end-to-end confirmation rpath-loaded ICU works.

### 5.4 Compat harness

- 5 new `bare-jammy-<NN>` x86_64 fixtures (one per PHP minor), `runner_os: ubuntu-22.04`.
- Existing 65 fixtures default to `runner_os: ubuntu-24.04` — they are the primary proof that jammy-built bundles load on noble, because every one of them consumes a freshly-rebuilt (jammy-built, hermetic-capable) bundle.
- `hermetic-audit` pre-flight job gates `ours`/`theirs` per fixture.
- No allowlist growth expected; specifically the ICU / ImageMagick / libltdl misses that would otherwise appear on the noble runner set are eliminated by hermetic capture.

### 5.5 Cross-slice regression

- C1's 5 `bare-arm64-<NN>` fixtures continue on `ubuntu-24.04-arm`.
- C2a's 5 `multi-ext-top14-arm64-<NN>` + C2b's 5 `multi-ext-hard4-arm64-<NN>` continue on `ubuntu-24.04-arm`.
- All load jammy-rebuilt hermetic-capable bundles; failures surface here if arm64 capture has any builder quirk.

### 5.6 Local development

`make bundle-php PHP_VERSION=8.4 ARCH=x86_64` uses `ubuntu:22.04` Docker image. `make bundle-ext` same. `Makefile` targets updated. `CONTRIBUTING.md` gains a "hermetic libs" section describing how to add a glob and audit locally.

### 5.7 Coverage

Per `CLAUDE.md`: 80% per-package. New `cmd/hermetic-audit` held from day one. `internal/catalog` field addition doesn't widen coverage meaningfully. `internal/planner` gains small branches + matching tests.

## 6. Release

- **Single PR `phase2-cross-os-hermetic-libs`.** Rebased onto `origin/main` before every push.
- **Conventional commits:**
  - `feat(catalog): hermetic_libs field for cross-OS runtime lib capture`
  - `feat(builders): capture-hermetic-libs.sh + rpath LDFLAGS`
  - `feat(builders): pin BUILDER_OS to ubuntu-22.04 via builder-os.env`
  - `feat(planner): fold BUILDER_OS into ComputeSpecHash`
  - `feat(bundle): schema v3 with hermetic_libs sidecar`
  - `feat(hermetic-audit): ldd-based cross-OS audit tool`
  - `feat(workflows): migrate bundle builds to ubuntu-22.04 runners`
  - `feat(workflows): harness runner_os axis + pre-flight hermetic-audit`
  - `feat(compat): bare-jammy-<NN> fixtures`
  - `feat(catalog): enable hermetic_libs on php core (ICU) and imagick (MagickWand/Core/ltdl)`
  - `feat(phpup): trim imagick runtime_deps to match hermetic catalog`
  - `chore(lock)` — bot-committed 182-entry refresh.
  - `docs(bundle-schema-changelog): v3 — hermetic library capture`
  - `docs(readme): jammy+noble supported runners; drop noble-only caveat`
- **release-please** auto-cuts the next alpha on merge. `v0.2.0-alpha` exit-tag rename is orthogonal.
- **Bundle digest churn: 182 entries.** `bundles.lock` diff is ~180 structured lines.
- **Rollback:**
  - Full revert: rebuild schema-v2 bundles, revert `MinBundleSchema` to 2. Users on the new alpha tag fail fast until they bump; prior alpha users unaffected.
  - Partial (one extension's hermetic set is wrong): delete that extension's `hermetic_libs` block, single-cell rebuild (sub-10-minute pipeline run).
  - GHCR digest immutability means no data loss in any path.
- **Follow-up issues on merge:**
  - Close #51 (cross-OS Ubuntu — resolved by this slice).
  - Any slice-D follow-up imagick tracker (if filed — resolved here).
- **README** — updated to name supported runners (jammy + noble, x86_64 + aarch64); noble-only caveat removed.

## 7. Open questions

Resolved in-PR per standard playbook:

1. **ImageMagick's transitive graph.** `libltdl` declared defensively; the in-PR audit on noble confirms which of ImageMagick's transitive deps actually drift and prunes the list.
2. **ICU data files (`icudt*.dat`).** `libicudata.so.*` covers the compiled-in data DLL. If the audit surfaces a filesystem-path `.dat` dependency, a different capture mechanism is needed (expected non-issue given Ubuntu's libicu build; verify during the audit).
3. **libstdc++ ABI forward-compat.** ICU 70 built against jammy's libstdc++ loaded on noble's libstdc++ — C++ ABI is stable by policy, audit confirms.
4. **`ldd` inside Docker vs runner's actual apt state.** The Docker images match the runner's apt universe by default. Runner-installed extra apt state that the image doesn't have is a known limitation of the audit, documented.
5. **`patchelf` as contingency.** If link-time `-Wl,-rpath` doesn't propagate through a libtool-wrapped extension build, fall back to `patchelf --set-rpath` post-build. Contingency only.
6. **`--disable-intl` as emergency escape.** If ICU bundling reveals a showstopper, short-term escape is `--disable-intl` + ship intl as a separately-built extension. Not planned; kept in reserve.

## 8. References

- `docs/product-vision.md` §6.1 (bundle definition including "statically-linked runtime libraries").
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4 (Phase-2 scope).
- `docs/superpowers/specs/2026-04-22-phase2-ubuntu-22.04-design.md` (rolled-back slice D; superseded by this spec).
- `docs/superpowers/specs/2026-04-21-phase2-aarch64-c1-design.md` (runner-OS-aware fixture schema precedent).
- `docs/superpowers/specs/2026-04-20-compat-harness-design.md` (harness integration reference).
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` (schema-version contract).
- GitHub issue #51 (cross-OS Ubuntu support — this slice closes it).
- `ld(1)`, `ld.so(8)` — rpath / DT_RUNPATH semantics.
- `patchelf(1)` — contingency reference.
- `CLAUDE.md` — quality gates, commit style, coverage target.
