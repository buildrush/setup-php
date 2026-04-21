# Phase 2 — Hard-tier PECL Extensions (slice B2)

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (follow-up slice from `2026-04-17-phase2-compat-slice-design.md` §8 item 3 "Extension matrix expansion to top-50"; splits the PECL-bundle half of §4.4 into B1 + B2)
**Target release:** `v0.2.0-alpha.8` on merge

## 1. Summary

Slice B1 (PR #42, merged) added the 10 easy-and-medium-complexity PECL extensions. This slice closes out the hard tier: `imagick`, `mongodb`, `swoole`, `grpc`. These four were split out because each has bespoke failure surface worth diagnosing in isolation — ImageMagick 6-vs-7 linkage, mongodb's OpenSSL/SASL2 dependency chain, swoole's large configure matrix, grpc's 30–45 minute compile time — but none of that actually changes the catalog shape or build pipeline. This is a pure data slice that inherits every piece of scaffolding from B1 (`build_deps.linux`, `runtime_deps.linux`, `installRuntimeDeps`, `BUILD_DEPS` env).

Scope math: 4 extensions × 5 PHP versions, minus swoole's metadata exclude on 8.1 (swoole 6.2.0 declares `<min>8.2.0</min>`) = **19 new bundles**. Five new harness fixtures (`multi-ext-hard4-<NN>`, one per version, `-81` variant omits swoole). No Go code, no builder script, no workflow changes.

On merge, the bundle inventory reaches 91 entries (5 cores + 86 ext bundles), matching `shivammathur/setup-php@v2`'s top-18 PECL list in full across every built PHP version except where upstream compatibility genuinely blocks (redis@8.5, igbinary@8.5, protobuf@8.1, swoole@8.1).

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `ext:<name>:<pinned-version>:<phpMinor>:linux:x86_64:nts` entries for the 4 new extensions across 8.1–8.5, minus swoole@8.1 (and any further per-extension excludes from in-PR audit).
- `catalog/extensions/` gains 4 new YAML files following the B1 shape.
- `cmd/phpup/main.go`'s runtime catalog map grows by 4 entries, with `RuntimeDeps` matching each YAML's `runtime_deps.linux`.
- `test/compat/fixtures.yaml` has 5 new `multi-ext-hard4-<NN>` fixtures (one per PHP version). `multi-ext-hard4-81` uses only `imagick, mongodb, grpc`; the others use all 4.
- Compat harness passes all 50 fixture pairs (45 existing + 5 new) with no new allowlist entries beyond the existing `env_delta` / `extensions` / `path_additions` bands. Per-extension ini drift that does appear is resolved in-PR per the bundle-rollout PR-self-publish flow.

### Non-goals
- Any structural change to builder/workflow/runtime. All machinery is inherited from B1.
- PECL version bumps on the existing 14 extensions. Single-purpose slices per project convention.
- Per-extension single-ext harness fixtures. A shared `multi-ext-hard4` per version is the primary compat check; per-extension smoke tests at build time remain the isolation check.
- Bundling runtime libraries inside extension bundles (aspirational per product-vision §6.1; deferred).
- ZTS, macOS/Windows, aarch64, ubuntu-22.04 — remain scoped to later phases / slices.
- Generating `cmd/phpup/main.go`'s runtime catalog map from `catalog/extensions/` at build time. Still tech debt; same policy as B1.

## 3. Source of truth

Same pinning as slice #1 and all subsequent Phase-2 slices:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` |

### 3.1 Extension audit (this slice)

Performed 2026-04-21 via PECL `package.xml` `<dependencies>/<php>`:

| Extension | Pinned | Min PHP | Max PHP | 8.1 | 8.2 | 8.3 | 8.4 | 8.5 |
|---|---|---|---|---|---|---|---|---|
| imagick | 3.8.1 | 5.6.0 | — | ✓ | ✓ | ✓ | ✓ | ✓ |
| mongodb | 2.2.1 | 8.1.0 | 8.99.99 | ✓ | ✓ | ✓ | ✓ | ✓ |
| swoole | 6.2.0 | 8.2.0 | 8.5.99 | **excluded** | ✓ | ✓ | ✓ | ✓ |
| grpc | 1.80.0 | 7.0.0 | — | ✓ | ✓ | ✓ | ✓ | ✓ |

Only swoole has a metadata-driven exclude. Runtime build failures surfaced in CI get `exclude: { php: "X.Y" }` per the standard pattern.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/extensions/imagick.yaml` | New. `kind: pecl`, `versions: ["3.8.1"]`, `abi_matrix.php: ["8.1"..."8.5"]`, `build_deps.linux: ["libmagickwand-dev", "libmagickcore-dev"]`, `runtime_deps.linux: ["libmagickwand-6.q16-6t64", "libmagickcore-6.q16-6t64"]`, `ini`, `smoke`. |
| `catalog/extensions/mongodb.yaml` | New. `build_deps.linux: ["libssl-dev", "libsasl2-dev"]`, `runtime_deps.linux: ["libssl3t64", "libsasl2-2"]`. |
| `catalog/extensions/swoole.yaml` | New. `abi_matrix.php: ["8.1"..."8.5"]`, `exclude: [{ php: "8.1" }]` (per §3.1), `build_deps.linux: ["libssl-dev", "libcurl4-openssl-dev"]`, `runtime_deps.linux: ["libssl3t64", "libcurl4t64"]`. |
| `catalog/extensions/grpc.yaml` | New. `build_deps.linux: ["zlib1g-dev", "libssl-dev"]`, `runtime_deps.linux: ["libssl3t64"]`. PECL grpc ships its own bundled grpc/protobuf/c-ares; no `libgrpc-dev` system dep needed up front. |
| `cmd/phpup/main.go` (runtime catalog map) | Grow hardcoded `Extensions` map from 14 entries to 18. Each new entry lists `Name`, `Kind: catalog.ExtensionKindPECL`, `Versions`, and `RuntimeDeps` exactly matching the YAML's `runtime_deps.linux`. |
| `test/compat/fixtures.yaml` | 5 new fixtures: `multi-ext-hard4-{81,82,83,84,85}`. `-81` variant extensions list: `'imagick, mongodb, grpc'` (no swoole). `-82` through `-85`: `'imagick, mongodb, swoole, grpc'`. |
| `docs/compat-matrix.md` | No preemptive edits. Per-extension ini drift surfaced in CI gets a per-extension allowlist entry with documented justification (same pattern as existing xdebug/disable_functions). |
| `internal/catalog`, `internal/resolve`, `internal/compose`, `cmd/planner`, `builders/linux/build-ext.sh`, `.github/workflows/*`, rest of `internal/` | **Unchanged.** |
| `README.md` | Not edited. |

### 4.2 Known unknowns (resolved per-extension in CI)

1. **imagick 6 vs 7.** Noble ships ImageMagick 6.x via `libmagickwand-dev`. imagick 3.8.x targets both 6 and 7 via compile-time feature detection. If the 6.x path fails on noble, fallback options are (a) pin an older imagick version that explicitly supports 6, or (b) install IM7 from a PPA. Both are follow-up slices; the in-PR response is `exclude:` and issue creation.
2. **grpc compile time.** Each cell ~30–45 min. matrix-parallel so ≤45 min wall-clock. Existing `max-parallel` throttle in `build-extension.yml` / `plan-and-build.yml` applies. No spec-level mitigation needed; monitor CI minutes.
3. **grpc PECL source mode.** The PECL distribution ships its own bundled grpc/protobuf/c-ares sources that compile inline. If a bundled-source compile fails (e.g. noble's gcc emits a warning treated as error), fall back: add `libgrpc-dev` `libprotobuf-dev` `protobuf-compiler` to `build_deps.linux` and set env `GRPC_BUILD_ENABLE_CCACHE=1` or similar to use system libs. Unlikely for grpc-1.80; documented as fallback only.
4. **swoole configure surface.** The default configure for swoole produces a minimal build that's fine for basic CI use. Opt-in features (HTTP/2, OpenSSL, PostgreSQL, JSON SIMD) need `--enable-*` flags. Build with defaults; if users request opt-in features, those become separate slices.
5. **mongodb-ext SSL binding.** mongodb 2.2.1 links against system OpenSSL via libssl-dev. Noble's libssl3t64 is compatible. No known issue.

### 4.3 Per-extension YAML shapes

Each file mirrors the B1 shape exactly. The specific fields are enumerated in §4.1 above; `ini: ["extension=<name>"]` and `smoke: ['php -r "assert(extension_loaded(\"<name>\"));"']` are uniform across the 4.

### 4.4 Runtime catalog map growth

`cmd/phpup/main.go` gains 4 entries matching the B1 pattern (hand-edited; tech-debt tracked but not fixed in this slice). The map after this slice lists 18 extensions — complete coverage of `phased-implementation-design.md §4.4` PECL tier.

## 5. Testing

### 5.1 Unit tests

No new unit tests required. `installRuntimeDeps` and `BuildDeps` parsing are already covered by B1's tests; those continue to pass unchanged.

### 5.2 Per-extension build-time smoke

Each new YAML has `smoke: ['php -r "assert(extension_loaded(\"<name>\"));"']`. Catches ABI mismatch, missing runtime dep, wrong path — in isolation, per PHP version. Primary regression check.

### 5.3 Harness fixtures

5 new `multi-ext-hard4-<NN>` fixtures. Shared-multi-ext strategy — one fixture loads the full hard-tier set per version. Cross-version regression is automatic via the existing fixture-matrix.

### 5.4 CI cost

~19 × 15-45 min = 5-14 hr aggregate, matrix-parallel to ~45 min wall-clock dominated by grpc. Harness cost grows from 135 cells to 150 (50 fixtures × 3 jobs).

### 5.5 Coverage

Per `CLAUDE.md`: 80% per-package. This slice ships no new production code; coverage numbers held by construction.

## 6. Release

- **Single PR, branch `phase2-hard4-ext`.** PR-self-publish per the bundle-rollout invariant.
- **Conventional commits**:
  - `feat(catalog): add hard-tier PECL extensions (phase 2 B2)`
  - `feat(phpup): extend runtime catalog with hard-tier PECL extensions`
  - `test(compat): add multi-ext-hard4 harness fixtures`
  - `chore(lock)` — bot-committed after pipeline.
- **release-please** — patch alpha, `v0.2.0-alpha.8`. Phase-2-exit `v0.2.0-alpha` (without dot-suffix) still waits on slices C (aarch64) and D (ubuntu-22.04).
- **Rollback** — per-extension break: `exclude:` on the affected PHP version or whole extension; YAML removal for a full rollback. Same mechanics as B1.
- **README** — not touched in this slice.

## 7. Open questions

Resolved per-PR:

1. **imagick ImageMagick 6/7.** If noble's IM6 fails, `exclude:` and open a follow-up issue for IM7 or imagick-version-bump.
2. **grpc bundled-source build.** If PECL grpc doesn't compile cleanly on noble, add `libgrpc-dev` + `libprotobuf-dev` to `build_deps.linux`. Follow-up slice addresses the added runtime deps.
3. **swoole default configure.** Ships with minimal features. If the harness surfaces compat drift against v2's richer swoole build, either enable matching flags or add allowlist entries documenting the delta.
4. **Harness ini drift for new extensions.** v2 installs these extensions via apt packages (e.g. `php-imagick`, `php-mongodb`); ini defaults may differ from source-built. Per-extension allowlist entries resolve.
5. **grpc build-minute burn.** If 45-min cells become cost-prohibitive, consider pinning an older grpc with faster build (trade-off: older grpc = older PHP compat window). Not solved here.

## 8. References

- `docs/product-vision.md` §6.1 (bundle kinds), §15 (long-tail strategy).
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4.4 (top-50 extension list; this slice closes the PECL half).
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 item 3.
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` (PR-self-publish invariant).
- `docs/superpowers/specs/2026-04-21-phase2-top10-ext-design.md` (slice B1, predecessor).
- `docs/compat-matrix.md` — v2 baseline + deviations allowlist.
- `CLAUDE.md`.
- Issues: #38 (redis@8.5 bump), #43 (igbinary@8.5 bump).
