# Phase 2 — PHP Version Expansion: 8.1 / 8.2 / 8.3 / 8.5 on linux/x86_64

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (follow-up slice #2 from `2026-04-17-phase2-compat-slice-design.md` §8)
**Target releases:** `v0.2.0-alpha.3` → `v0.2.0-alpha.6` (one patch alpha per PR)

## 1. Summary

Phase 2 of `buildrush/setup-php` is an umbrella for Linux matrix expansion. Slice #1 (`2026-04-17-phase2-compat-slice-design.md`) landed v2 input parity on PHP 8.4/x86_64. The compat harness (`2026-04-20-compat-harness-design.md`), compat closeout (`2026-04-20-phase2-compat-closeout-design.md`), and bundle-rollout (`2026-04-20-bundle-schema-and-rollout-design.md`) closed the behaviour and infrastructure gaps.

This spec covers the next slice: extending the built matrix from `{8.4}` to `{8.1, 8.2, 8.3, 8.4, 8.5}` on the same `linux / x86_64 / nts / ubuntu-24.04` cell. aarch64, ubuntu-22.04, and top-50 extensions remain deferred to later slices.

The slice ships as **four sequential PRs, one per new PHP minor, in order 8.5 → 8.3 → 8.2 → 8.1**. Each PR is self-contained per the PR-self-publish invariant from the bundle-rollout design: it flips on `sources:` for its version, rebuilds the 4 existing PECL extensions against that ABI, adds 6 compat-harness fixtures, and self-publishes the resulting 5 bundles to GHCR with the lockfile committed back to the PR head.

After all four PRs land, `bundles.lock` holds 25 entries (5 cores + 20 extension bundles = 4 PECL extensions × 5 PHP versions) and the compat harness runs 30 fixture pairs against a pinned `shivammathur/setup-php@v2` SHA.

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `php:{8.1,8.2,8.3,8.5}:linux:x86_64:nts` plus `ext:{redis,xdebug,pcov,apcu}:{pinned-version}:{8.1,8.2,8.3,8.5}:linux:x86_64:nts` entries, all self-published by the PR that introduces them per the bundle-rollout invariant.
- `catalog/php.yaml` has a self-contained build block (`sources`, `abi_matrix`, `configure_flags.common`, `configure_flags.linux`) for each of 8.1 / 8.2 / 8.3 / 8.5, with the flag set audited against that version's `bundled_extensions` catalog entry and against `internal/compat.BundledExtensions`.
- `internal/compat.DefaultIniValues`, `XdebugIniFragment`, and `BundledExtensions` are golden-file-tested for every cataloged PHP version (all five), not just 8.4.
- `test/compat/fixtures.yaml` holds 30 fixtures (6 × 5). The compat-harness workflow goes green across all pairs with no new allowlist entries beyond the existing `env_delta` / `extensions` / `path_additions` bands (which are Phase 3 scope).
- The `sources:`-gated planner behavior is preserved: adding a `sources:` block triggers builds for that version; removing it (e.g. for a rollback) stops future builds without invalidating already-published digests referenced by a pinned action release.

### Non-goals
- Top-50 extension expansion — slice B of the Phase 2 umbrella.
- aarch64 — slice C. `DefaultIniValues` stays single-arch per version; the per-arch branch called out in the closeout spec §7 is deferred to slice C.
- ubuntu-22.04 runner coverage — slice D.
- `tools:` input — Phase 3.
- Tier 2/3/4 fallbacks — Phase 3.
- PHP 7.x or 8.0 — explicitly excluded from Phase 2.
- Bumping the `shivammathur/setup-php@v2` SHA or Ondrej PPA snapshot date. `compat-matrix.md` stays at the slice-#1 pinning; drift audit is a separate maintenance concern.
- PHP 8.5 nightly / master — the catalog's 8.5 block tracks the current 8.5 GA release (or latest RC if GA has not yet landed at PR-1 time). Nightly tracking stays tier-3 per product-vision §15.
- Bumping PECL extension versions (redis, xdebug, pcov, apcu). Each PR uses the version currently pinned; if an ABI compatibility gap is discovered, the fix is an `exclude:` entry, not a version bump. Version bumps are their own single-purpose slice.

## 3. Source of truth

Same pinning as slice #1 and all subsequent Phase 2 slices:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` (unchanged — no new audit is performed in this slice) |

Compat-matrix sections consulted:
- §2.1 (default ini values — all 8.x)
- §2.2 (xdebug ini fragment)
- §5.6 / §5.7 / §5.8 (coverage side-effects; already implemented by closeout)
- Per-version `bundled_extensions` tables (slice-#1 data)

## 4. Architecture

The slice is data + builder-exercise only. **No runtime-code changes.**

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/php.yaml` | Each new version block gains `sources`, `abi_matrix`, `configure_flags.common`, `configure_flags.linux` — self-contained, duplicated deliberately (see §4.2). `bundled_extensions` lists stay as-is (already correct per slice #1). |
| `catalog/extensions/{redis,xdebug,pcov,apcu}.yaml` | `abi_matrix.php` extended from `["8.4"]` to `["8.1","8.2","8.3","8.4","8.5"]` (with any `exclude:` entries discovered in the §4.3 audit). Extension versions stay pinned. |
| `builders/linux/build-php.sh` | No structural change. The script reads `configure_flags` from the active version block; the tarball URL template (`{version}`) already parameterises across versions. Behaviour-verify on each version during PR-N. |
| `builders/linux/build-ext.sh` | No structural change. Matrix expansion is automatic once `abi_matrix` grows. |
| `internal/compat/compat.go` | **No code change.** `DefaultIniValues` branches on `isPHP8x(minorOf(phpVersion))` which already includes 8.1–8.5; `XdebugIniFragment` gate `xdebug3Supported` already switches on every 8.x minor (per compat-matrix §2.2); `BundledExtensions` is already version-indexed per slice #1. |
| `internal/compat/compat_test.go` | New golden-file cases for `DefaultIniValues` and `XdebugIniFragment` at 8.1 / 8.2 / 8.3 / 8.5. Existing 7.3, 8.4, and 9.0-boundary cases stay. |
| `internal/catalog/catalog_compat_test.go` | Already iterates every version in `catalog/php.yaml` and cross-checks `bundled_extensions` against `compat.BundledExtensions`. The set of versions validated grows automatically as each PR flips on `sources:`. No test code change. |
| `test/compat/fixtures.yaml` | 6 new fixtures per PR — a copy of the existing 8.4 set with `php-version:` swapped and the `name:` suffixed (`-81`, `-82`, `-83`, `-85`). |
| `.github/workflows/compat-harness.yml` | No workflow change. Matrix is generated from `fixtures.yaml` (harness design §5.6). |
| `.github/workflows/plan.yml`, `build-php-core.yml`, `build-extension.yml`, `on-push.yml` | No change. PR-self-publish flow (bundle-rollout PR-α / β.1 / β.2) handles new matrix cells automatically. |
| `cmd/phpup/main.go`, `internal/plan`, `internal/resolve`, `internal/compose`, `internal/oci`, `internal/extract`, `internal/cache`, `internal/env`, `cmd/planner`, `cmd/compat-diff`, `cmd/lockfile-update` | **Unchanged.** |
| `docs/compat-matrix.md` | No edit — §2 tables already version-aware; §5 dispositions unaffected. |
| `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` | Phase 2 roadmap annotation: per-PR checkbox block linking back to this design; added in PR-1 (8.5), ticked as each PR lands. |
| `README.md` | Compatibility section updates from "PHP 8.4 built" to "PHP 8.1–8.5 built" in the **last** PR of the series so the doc and reality land together. |

### 4.2 Per-version configure-flag duplication (chosen option)

Two options were considered:

- **4.2-A — Duplicate `configure_flags` into each version block.** Chosen. Each version block is self-contained; per-version drift is visible in the diff; no shared merge semantics.
- **4.2-B — Hoist a top-level `configure_flags.common` with per-version overrides.** Rejected: introduces merge semantics (override? append? per-flag?) that the planner has to encode and tests have to pin, for a set of five finite, manually-curated blocks.

Mechanics under 4.2-A:

1. The catalog block for each new version carries its own `configure_flags.common` and `configure_flags.linux`. In most cases the strings are identical to 8.4's; per-version drift lives explicitly in the diff.
2. The builder script reads `configure_flags` from the active version block; no script-side branching on version.
3. A truly-common flag change touches 5 places. This is acceptable given the finite version count — 5 blocks is not a refactor smell, and the duplication keeps the single source of truth local to each build.

Per-version delta informed by each version's `bundled_extensions` block (resolved concretely in each PR's audit):

- **8.5**: may need flags for `lexbor` and `uri` (new bundled extensions in 8.5). If those are compile-default on 8.5's configure without a `--with-*` flag, the 8.4 common set applies unchanged.
- **8.2 / 8.3**: identical bundled-extension list to 8.4 minus any 8.4-specific additions (there are none vs 8.2/8.3). The 8.4 common flag set applies as-is.
- **8.1**: has no `random` (PHP 8.2+); `random` is default-on from 8.2 with no user-facing flag change, so its absence in 8.1 requires no flag removal.

### 4.3 Extension ABI compatibility audit

One-time check per PR, performed before the PR opens and captured as a table in the PR description:

| Extension | Pinned version | 8.1 | 8.2 | 8.3 | 8.4 | 8.5 |
|---|---|---|---|---|---|---|
| redis | 6.2.0 | audit in PR-4 | audit in PR-3 | audit in PR-2 | ✓ built | audit in PR-1 |
| xdebug | 3.5.1 | audit in PR-4 | audit in PR-3 | audit in PR-2 | ✓ built | audit in PR-1 |
| pcov | 1.0.12 | audit in PR-4 | audit in PR-3 | audit in PR-2 | ✓ built | audit in PR-1 |
| apcu | 5.1.28 | audit in PR-4 | audit in PR-3 | audit in PR-2 | ✓ built | audit in PR-1 |

Source for each audit: the extension's PECL metadata (`min_php_version` / `max_php_version`) and its release notes. If any cell is unsupported, the fix is an `exclude:` entry on that extension's `abi_matrix` — **not** a version bump.

### 4.4 Lockfile growth

After all 4 PRs land, `bundles.lock` grows from 5 entries to 25:

```
php:8.1:linux:x86_64:nts       + 4 ext entries @ 8.1    (PR-4, last)
php:8.2:linux:x86_64:nts       + 4 ext entries @ 8.2    (PR-3)
php:8.3:linux:x86_64:nts       + 4 ext entries @ 8.3    (PR-2)
php:8.4:linux:x86_64:nts       (unchanged)
ext:{redis,xdebug,pcov,apcu}:…:8.4:…  (unchanged)
php:8.5:linux:x86_64:nts       + 4 ext entries @ 8.5    (PR-1, first)
```

Each PR writes its own 5 entries via the commit-back flow. Version keys are disjoint across PRs; no cross-PR lockfile merge conflicts occur in normal operation.

### 4.5 PR ordering rationale

Four sequential PRs, in order **8.5 → 8.3 → 8.2 → 8.1**:

1. **8.5 first (PR-1).** Newest — most likely to surface the per-version `configure_flags` structural refactor first (e.g. `lexbor` / `uri` in bundled_extensions). Land the structural pattern against the hardest cell first; subsequent PRs are copy-adapt.
2. **8.3 (PR-2) then 8.2 (PR-3).** Middle; adjacent to the already-built 8.4; least likely to surface new compat drift.
3. **8.1 last (PR-4).** Oldest, nearest EOL (active support ended 2025-12-31, security-only until 2026-12-31); if cracks show, they show here. Landing 8.1 last means the slice is still shippable-as-partial if 8.1 hits an unresolvable compat issue.

Each PR depends only on the catalog/compat/bundle infrastructure that already exists; there are no ordering dependencies in the code between the PRs. The ordering is purely a risk-triage heuristic.

## 5. Testing

### 5.1 Unit tests (per PR)

- **`internal/compat`** — new golden-file cases:
  - `DefaultIniValues("8.1")` / `("8.2")` / `("8.3")` / `("8.5")` — assert the 8.x opcache/JIT block is present for all five; existing non-8.x boundary test stays.
  - `XdebugIniFragment` at each added version — assert `{"xdebug.mode": "coverage"}`; existing 7.3 and out-of-range cases stay.
  - `BundledExtensions` tests — already present per slice #1; no change.
- **`internal/catalog`** — `catalog_compat_test.go` iterates every version in `catalog/php.yaml` and cross-checks `bundled_extensions` against `compat.BundledExtensions`. The set of validated versions grows automatically as each PR flips on `sources:`.

### 5.2 Smoke tests (per PR)

- `test/smoke/run.sh` already asserts `php.ini-production` / `php.ini-development` presence (closeout slice). Same assertions apply unchanged.
- The catalog's `smoke:` block (`php -v`, `php -m`, `echo PHP_VERSION`) runs against each new core bundle at build-time via `build-php-core.yml`.
- Per-ABI extension smoke (`assert(extension_loaded("redis"))` etc.) runs automatically once `abi_matrix` expands.

### 5.3 Compat harness (per PR)

6 new fixtures per PR, each a copy of the existing 8.4 set with `php-version` swapped and the `name:` suffixed (`bare-85`, `coverage-xdebug-85`, `coverage-pcov-85`, `ini-file-development-85`, etc.). The matrix grows by 6 pairs per PR; no workflow change.

**Gate:** the harness must go green on the new pairs with zero new allowlist entries beyond the existing `env_delta` / `extensions` / `path_additions` bands. Any other deviation is a bug in this slice, not an allowlist growth target.

The `shivammathur/setup-php@v2` side of the harness pulls from the PPA snapshot pinned in `compat-matrix.md`. Older PHP versions on that PPA are stable, so each fixture pair is reproducible run-to-run.

### 5.4 Cross-PR regression check

After each PR lands, the harness for previously-landed versions must stay green. This is automatic (matrix is fixture-list-driven), but is called out as a gate: if PR-3 (8.2) goes red on the 8.5 fixtures from PR-1, that's a regression in shared catalog/compat code, not a new-version problem.

### 5.5 Integration test workflow

`integration-test.yml` — no change required in this slice. It currently tests 8.4; expansion to multiple versions is desirable but is out of scope (the compat harness is the stronger check and already multiplies across versions).

### 5.6 Coverage

Per `CLAUDE.md`: 80% per-package. This slice adds only test code and catalog data; no production-code changes means no coverage regression risk. Existing per-package numbers are held by construction.

### 5.7 Local development

`make bundle-php PHP_VERSION=8.1` already works — the builder script reads configure flags from the active catalog block. Contributors can rebuild any version locally via Docker once its catalog entry has `sources:`.

## 6. Release

- **Conventional commits per PR.** `feat(catalog): enable PHP 8.5 core + PECL rebuild`; `test(compat): add 8.5 golden files`; `test(compat): add 8.5 harness fixtures`. Similar per subsequent PRs.
- **release-please cadence.** Each PR is a `feat:` and cuts a patch alpha: `v0.2.0-alpha.3` (PR-1), `.4` (PR-2), `.5` (PR-3), `.6` (PR-4). The Phase-2-exit release `v0.2.0-alpha` without dot-suffix is cut **after** slice B (top-50 extensions) lands, per the original §8 roadmap. This slice does not bump the alpha major.
- **Lockfile flow.** Each PR self-publishes 5 bundles via the PR-α commit-back flow. No bot-PR path; no cross-PR lockfile merges (version keys are disjoint).
- **Rollback.** A published bundle is immutable by digest; a regrettable PR is rolled back by reverting the lockfile delta and removing the `sources:` block from that version's catalog entry. Future pipeline runs skip rebuild (no `sources:`); `gc-bundles.yml` reaps orphan digests after `GC_MIN_AGE_DAYS`.
- **README.** Compatibility section updates to list 8.1–8.5 as built (from just 8.4) in the **last** PR so doc and reality land together.
- **Phase-map annotation.** `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4.3 gets a per-PR checkbox block in PR-1, ticked as PRs land.

## 7. Open questions

Left for in-PR resolution; the design does not need to answer them upfront:

1. **PHP source signature verification.** `php.net` rotates release-manager GPG keys per major branch. `builders/linux/build-php.sh` today verifies against the 8.4 manager's key; older versions have different managers. The fix is a keyring refresh in the builder; whether to add every manager's key to a bundled keyring or fetch per-version at build time is a PR-N decision and surfaces in whichever PR first builds under a different manager.
2. **Ubuntu 24.04 build-dep drift on older PHP.** `libzip-dev`, `libssl-dev`, `libonig-dev` on noble may be newer than what PHP 8.1 upstream regression-tests against. If `./configure` or `make` fails on 8.1 against noble's deps, the fix is either (a) pinning a build-time apt snapshot or (b) using a narrower base image for the older cell (which overlaps with slice D). Decide in PR-4 if it actually breaks — not speculatively.
3. **xdebug 3.5.1 vs PHP 8.5.** Xdebug 3.5.x targets 8.0–8.5; confirm during PR-1 audit. If a newer 3.5.x / 3.6.x is out that officially lists 8.5 compatibility, a version bump is its own single-purpose slice per §2 non-goals — a broken pair in PR-1 becomes an `exclude:` and a follow-up.
4. **Harness CI cost.** 30 fixture pairs × 2 runs (theirs + ours) ≈ 60 jobs. If GHA concurrency limits bite on `pull_request`, `max-parallel` throttle on the harness workflow matrix is the knob (orthogonal to this slice's deliverables).
5. **`docs/compat-matrix.md` per-minor drift.** Slice #1 pinned ini-defaults "per 8.x"; if v2 has per-minor drift the slice-#1 audit did not catch (e.g. 8.1 jit defaults differ from 8.5), the harness detects it and forces an audit addendum. No speculative doc edit in this slice.

## 8. References

- `docs/product-vision.md` — overall product rationale and compat targets.
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` — Phase 2 scope; this slice is follow-up #2 per §4.
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` — slice #1 (compat-first); §8 enumerates this slice as follow-up item #2.
- `docs/superpowers/specs/2026-04-20-compat-harness-design.md` — harness workflow; §5.6 confirms data-only fixture expansion.
- `docs/superpowers/specs/2026-04-20-phase2-compat-closeout-design.md` — slice #2 (closeout); §7 defers `DefaultIniValues` per-arch branch to slice C (aarch64), keeping it out of scope here.
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` — PR-self-publish invariant; every PR in this slice follows it.
- `docs/compat-matrix.md` — §1.1, §2.1, §2.2, per-version `bundled_extensions` tables.
- `CLAUDE.md` — quality gates (`make check`), commit style, coverage target.
