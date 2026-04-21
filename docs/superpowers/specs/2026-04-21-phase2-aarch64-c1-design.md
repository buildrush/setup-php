# Phase 2 — Slice C1: aarch64 Infrastructure + PHP Cores

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (slice C1 of the Phase-2 umbrella; follow-up from `2026-04-17-phase2-compat-slice-design.md` §8 item 4 "aarch64")
**Target release:** `v0.2.0-alpha.9` on merge

## 1. Summary

Phase 2's umbrella roadmap (per `2026-04-17-phase2-compat-slice-design.md` §8 item 4) calls for aarch64 support. This is the first multi-arch slice in the repo: every bundle so far has targeted linux/x86_64 implicitly or explicitly. This slice (C1) lands the aarch64 **infrastructure** — workflow runner mapping, per-arch ini branching in `internal/compat`, catalog `abi_matrix` expansion, a minimal fixture set — and builds **only the 5 PHP cores** on aarch64. PECL extensions on aarch64 are deferred to slice C2, which becomes a pure data-expansion slice inheriting C1's scaffolding.

The split matches the successful B1/B2 pattern: new infrastructure lands in a small reviewable PR, catalog expansion follows in the next PR with zero structural churn. After C1, an arm64 user running `uses: buildrush/setup-php` with just `php-version: '8.4'` gets a working core; asking for `extensions: redis` on arm64 returns a resolver error (no `ext:redis:…:linux:aarch64:nts` entry yet). C2 closes that gap.

Infrastructure changes are narrow:
- `.github/workflows/build-php-core.yml` and `build-extension.yml` — map `arch == "aarch64"` to `runs-on: ubuntu-24.04-arm` (free on public repos). Extension workflow change is scaffolding for C2; doesn't fire in C1.
- `builders/linux/build-ext.sh` — drop hardcoded `x86_64` in the `fetch-core.sh` call.
- `internal/compat.DefaultIniValues(phpVersion, arch)` — per compat-matrix §2.4, `opcache.jit_buffer_size=128M` on aarch64 (vs 256M on x86_64); all other keys unchanged.
- `test/compat/fixtures.yaml` — new optional `arch:` field (defaults to `x86_64`); 5 new `bare-arm64-<NN>` fixtures per PHP version.
- `compat-harness.yml` — per-fixture `runs-on` map.
- `catalog/php.yaml` — each version block's `abi_matrix.arch` grows from `["x86_64"]` to `["x86_64", "aarch64"]`.

On merge, `bundles.lock` holds 96 entries (current 91 + 5 arm64 cores). `fixtures.yaml` holds 55 fixtures (165 harness cells per CI run vs current 150). Action release `v0.2.0-alpha.9`.

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `php:{8.1,8.2,8.3,8.4,8.5}:linux:aarch64:nts` entries, self-published by the PR per the bundle-rollout invariant.
- `internal/compat.DefaultIniValues` accepts `(phpVersion, arch string)` and branches `opcache.jit_buffer_size` correctly. Existing callers updated to pass `Plan.Arch` (already resident).
- `builders/linux/build-ext.sh` uses the caller-provided `$ARCH` when fetching the core bundle, eliminating the hardcoded `x86_64`. Existing behaviour preserved when `$ARCH` unset (defaults to `x86_64`).
- `build-php-core.yml` and `build-extension.yml` dispatch to `ubuntu-24.04-arm` when `inputs.arch == "aarch64"`. Existing x86_64 cells dispatch to `ubuntu-24.04` unchanged.
- `catalog/php.yaml`'s `abi_matrix.arch` expands to include `aarch64` for all 5 PHP minors.
- `test/compat/fixtures.yaml` has 5 new `bare-arm64-<NN>` fixtures with an optional `arch: 'aarch64'` field. The 50 existing fixtures remain x86_64-implicit.
- `compat-harness.yml` dispatches `ours` / `theirs` jobs to the right runner per fixture arch.
- `docs/compat-matrix.md` §2.4 moves from "unfetched" to audited; records `opcache.jit_buffer_size=128M` as the only aarch64 divergence.
- Compat harness passes 55 fixtures × 3 jobs = 165 cells with no new allowlist entries beyond existing bands.

### Non-goals
- PECL extensions on aarch64 — slice C2. All `catalog/extensions/*.yaml` stay `arch: ["x86_64"]`.
- ubuntu-22.04 — slice D.
- macOS / Windows — Phases 5 / 6.
- New harness fixture types (multi-ext, coverage, etc.) on aarch64 — if per-arch ini drift surfaces beyond `opcache.jit_buffer_size`, a later slice upgrades coverage.
- Changing the digest formula or bundle schema. Existing content-addressing already keys on arch per lockfile-key shape.
- Reworking `Plan.Arch` detection from `RUNNER_ARCH`. Detection already works per slice #1; we just consume the existing value.
- Per-arch `runtime_deps.linux` keying in extensions (covered by C2 / slice D if Ubuntu arm64 package names diverge).

## 3. Source of truth

Same pinning as every Phase-2 slice since #1:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` |

### 3.1 v2 `jit_aarch64.ini` audit (new, 2026-04-21)

Fetched from `https://raw.githubusercontent.com/shivammathur/setup-php/accd6127cb78bee3e8082180cb391013d204ef9f/src/configs/ini/jit_aarch64.ini`:

```
opcache.enable=1
opcache.jit_buffer_size=128M
opcache.jit=1235
```

Delta vs v2's x86_64 `jit.ini` (already encoded in `compat.DefaultIniValues`):

| Key | x86_64 | aarch64 |
|---|---|---|
| `opcache.enable` | `1` | `1` |
| `opcache.jit` | `1235` | `1235` |
| `opcache.jit_buffer_size` | `256M` | **`128M`** |

Only one key diverges. `DefaultIniValues`' per-arch branch is therefore a one-line conditional.

### 3.2 GitHub Actions arm64 runner availability

`ubuntu-24.04-arm` is free for public repos since 2025 Q1. Private repos pay. This project is public (`buildrush/setup-php`) — no billing impact.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/php.yaml` | Each `"8.1"` .. `"8.5"` block's `abi_matrix.arch` grows `["x86_64"]` → `["x86_64", "aarch64"]`. 5 blocks edited. |
| `.github/workflows/build-php-core.yml` | Replace `runs-on: ubuntu-24.04` with `runs-on: ${{ inputs.arch == 'aarch64' && 'ubuntu-24.04-arm' || 'ubuntu-24.04' }}`. |
| `.github/workflows/build-extension.yml` | Same `runs-on` map (scaffolding for C2; doesn't fire in C1 because ext abi stays x86_64). |
| `.github/workflows/compat-harness.yml` | `ours` and `theirs` jobs gain the same `runs-on` map keyed off `matrix.arch` (new fixture field; defaults x86_64). |
| `builders/linux/build-ext.sh` | Replace hardcoded `"$PHP_ABI" linux x86_64` call with `"$PHP_ABI" linux "$ARCH"` (and ensure `ARCH="${ARCH:-x86_64}"` defaulting is set at the top of the script like `build-php.sh`). |
| `builders/linux/build-php.sh` | **No change.** Already reads `$ARCH` (defaulting `x86_64`) and uses it in tag names. |
| `internal/compat/compat.go` | `DefaultIniValues(phpVersion string) map[string]string` → `DefaultIniValues(phpVersion, arch string) map[string]string`. When `isPHP8x(minor)` AND `arch == "aarch64"`, set `opcache.jit_buffer_size = "128M"`; else `"256M"`. All other keys and non-8.x branches unchanged. |
| `internal/compat/compat_test.go` | `TestDefaultIniValues_PHP8xOpcacheJIT` table grows with an `arch` column; assertions branch `jit_buffer_size` per arch. `TestDefaultIniValues_NonPHP8` adds `arch` param (no-op on the underlying code — below-8.x returns the same 2-key set regardless of arch). `TestDefaultIniValuesMatchesGolden` calls `DefaultIniValues("8.4", "x86_64")` matching the existing golden. |
| `internal/compose` (and any other caller) | Single caller site in `cmd/phpup/main.go` pulls `p.Arch` from the parsed Plan and passes it to `DefaultIniValues`. No behavioural change on x86_64 flows. |
| `cmd/phpup/main.go` | Update the `DefaultIniValues` call to pass `p.Arch`. No other change. |
| `internal/plan` | `Plan.Arch` already exists; no change required. Verify detection correctly produces `"aarch64"` when `RUNNER_ARCH=ARM64` (existing logic per slice #1; sanity-check only). |
| `test/compat/fixtures.yaml` | New optional `arch:` field. 5 new `bare-arm64-<NN>` fixtures (one per PHP minor): `arch: 'aarch64'`, `extensions: ''`, `coverage: 'none'`, `ini-values: ''`. |
| `test/compat/fixtures.yaml` parse (workflow) | The `yq | jq` matrix generator already carries every fixture field through to the matrix `include[]`; adding `arch` is passive unless the workflow consumes it (it does, via the `runs-on` map). |
| `docs/compat-matrix.md` | §2.4 "jit_aarch64.ini (arm64/aarch64 only)" goes from "Not fetched in this audit" to the three-key block recorded in §3.1 above. Disposition column on any ini-defaults rows in §5 that reference per-arch divergence updated to "implemented (slice C1)". |
| `cmd/phpup/main.go` runtime catalog map | **No change.** Extensions stay x86_64 in this slice. |
| `README.md` | Status line stays "Linux x86_64 only". Updated to "Linux x86_64 + aarch64" by C2 once extensions are there too. |

### 4.2 Runner runs-on map (chosen idiom)

GitHub Actions accepts `runs-on:` as an expression. The one-liner idiom:

```yaml
runs-on: ${{ inputs.arch == 'aarch64' && 'ubuntu-24.04-arm' || 'ubuntu-24.04' }}
```

Readable, no nested matrix config, and the JSON-map alternative (per product-vision §12.2) is heavier for a 2-arch case. If a third arch ever lands (x86_64/aarch64/arm64-windows etc.), promote to a map literal.

### 4.3 Fixture `arch` field semantics

New optional field `arch: string`. Absent → `x86_64`. `aarch64` → dispatch `ours` / `theirs` to `ubuntu-24.04-arm`. The matrix generator step in `compat-harness.yml` passes `arch` into the matrix row; both `ours` and `theirs` jobs read it from `matrix.arch`.

Existing 50 fixtures stay as-is; no migration churn.

### 4.4 `DefaultIniValues` signature change impact

Breaking change to the function. Callers:
1. `cmd/phpup/main.go` (one call site via the compose pipeline).
2. Tests in `internal/compat/compat_test.go`.

No external API surface — the function lives in `internal/`. Grep'd audit before implementation ensures every callsite is updated.

### 4.5 Lockfile key shape (no change)

Existing `php:<version>:<os>:<arch>:<ts>` and `ext:<name>:<version>:<phpMinor>:<os>:<arch>:<ts>` keys already carry `<arch>`. Adding aarch64 entries is natural expansion — no key-format migration.

### 4.6 Lockfile growth

After slice C1 lands, `bundles.lock` grows from 91 to 96:

```
(existing 91 entries unchanged)
php:8.1:linux:aarch64:nts
php:8.2:linux:aarch64:nts
php:8.3:linux:aarch64:nts
php:8.4:linux:aarch64:nts
php:8.5:linux:aarch64:nts
```

## 5. Testing

### 5.1 Unit tests

- `internal/compat`:
  - `TestDefaultIniValues_PHP8xOpcacheJIT` becomes a 2D table: `{arch} × {8.0, 8.1, 8.2, 8.3, 8.4, 8.5, 8.4.5, 8.9}`. Asserts `opcache.jit_buffer_size` = `256M` on x86_64, `128M` on aarch64; asserts other keys identical across archs.
  - `TestDefaultIniValues_NonPHP8` adds arch column — both `x86_64` and `aarch64` return the same 2-key set for 7.4 / 9.0.
  - `TestDefaultIniValuesMatchesGolden` — call with `("8.4", "x86_64")` to preserve the existing golden file. An aarch64 golden is not added (we have pinpoint tests for the one-key delta; golden file is redundant for the architecture-specific variant).

No other packages need new tests in this slice — the work is data + workflow dispatch, which the harness exercises end-to-end.

### 5.2 Smoke tests

- `catalog/php.yaml`'s `smoke:` block (`php -v`, `php -m`, `echo PHP_VERSION`) runs automatically on each new aarch64 core bundle at build time. Catches ABI mismatch, broken compile, missing bundled extension, per PHP minor per arch.

### 5.3 Harness

- 5 new `bare-arm64-<NN>` fixtures, one per PHP minor. Each fires `ours` and `theirs` on `ubuntu-24.04-arm`.
- `ours` uses the just-published aarch64 core; `theirs` installs via `shivammathur/setup-php@v2` which has native arm64 support.
- `diff` compares the flattened probes. The only expected per-arch ini delta is `opcache.jit_buffer_size` (256M → 128M); our `DefaultIniValues` now emits that. Diff should be clean.

### 5.4 Cross-arch regression

All 50 existing fixtures continue to run on amd64 (fixture `arch:` defaults to x86_64). No coverage loss.

### 5.5 Coverage

Per `CLAUDE.md`: 80% per-package. `internal/compat` gains a small arch branch with matching test coverage. Numbers held.

### 5.6 Local development

`make bundle-php PHP_VERSION=8.4 ARCH=aarch64` continues to work for cross-arch testing via Docker (qemu-user-static in the Docker image handles arm64 emulation). Contributors wanting native arm64 testing need an arm64 host or a GitHub Actions arm64 runner.

## 6. Release

- **Single PR, branch `phase2-aarch64-c1`.** PR-self-publish per bundle-rollout invariant.
- **Conventional commits**:
  - `feat(compat): per-arch DefaultIniValues (aarch64 jit_buffer_size=128M)`
  - `feat(catalog): enable aarch64 builds on all 5 PHP cores`
  - `feat(builders/linux): parametrize build-ext.sh arch`
  - `feat(workflows): runs-on mapping for aarch64 matrix cells`
  - `test(compat): add bare-arm64 harness fixtures + arch field`
  - `docs(compat-matrix): record audited jit_aarch64.ini`
  - `chore(lock)` — bot-committed after pipeline.
- **release-please** → `v0.2.0-alpha.9`. Phase-2-exit `v0.2.0-alpha` still waits on C2 and slice D.
- **Rollback** — per-version build failure: `exclude: { arch: "aarch64" }` in that `catalog/php.yaml` version block. Slice-wide rollback: revert the catalog change (extensions untouched, no cascade).
- **README** — not edited in this slice. C2 updates status line when extensions also work on arm64.

## 7. Open questions

Resolved in-PR:

1. **PHP 8.1 on arm64.** Oldest, security-only support. If configure/make fails on arm64 autoconf, apply `exclude: { arch: aarch64 }` and file a tracking issue modeled on #38 / #43.
2. **arm64 apt-dep parity.** Ubuntu's main package set is arch-uniform (e.g., `libmagickwand-dev` exists on both). If any build-dep diverges on arm64 (unlikely), per-arch `build_deps.linux` keying becomes a follow-up spec. Not expected here.
3. **v2 `jit_aarch64.ini` newer-PHP delta.** The audit fetched the pinned-SHA file; if PHP 8.5's opcache-jit behavior on arm64 diverged post-pinning, the harness detects it and forces an audit addendum. No speculative per-minor branch in `DefaultIniValues`.
4. **Harness cost.** 5 new fixtures × 3 jobs each = 15 new matrix cells, all on `ubuntu-24.04-arm`. arm64 runners are free for public repos; no billing concern.
5. **Plan.Arch detection on arm64 runner.** Existing logic reads `$RUNNER_ARCH`. When a user runs on arm64, `RUNNER_ARCH=ARM64` → our plan parser maps to `"aarch64"` (per slice #1 code). Sanity-checked in-PR; if mis-mapped, a tiny plan-layer fix lands in this same PR.
6. **Cross-compile vs native arm64 build for phpup.** Go already cross-compiles via `src/index.js`'s RUNNER_ARCH branch. No change needed; verify the release workflow publishes `phpup-linux-arm64` binary as part of C1 validation.

## 8. References

- `docs/product-vision.md` §6.1, §12.2 (multi-arch matrix shape).
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4.2, §4.3 (aarch64 key work items).
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 item 4 (umbrella roadmap entry).
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` (PR-self-publish invariant).
- `docs/superpowers/specs/2026-04-21-phase2-version-expansion-design.md` §7 open Q1 (flagged aarch64 opcache.jit branch; resolved here).
- `docs/superpowers/specs/2026-04-20-phase2-compat-closeout-design.md` §7 "aarch64 opcache.jit defaults" (flagged the need for per-arch branching; resolved here).
- `docs/compat-matrix.md` §2.4 (updated by this slice).
- `CLAUDE.md` — quality gates, commit style, coverage target.
