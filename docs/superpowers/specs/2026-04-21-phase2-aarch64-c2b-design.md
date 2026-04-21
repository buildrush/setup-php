# Phase 2 — Slice C2b: Hard-tier PECL Extensions on aarch64

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (slice C2b of the Phase-2 umbrella; direct arm64 parallel of B2 `2026-04-21-phase2-hard4-ext-design.md`)
**Target release:** `v0.2.0-alpha.11` on merge

## 1. Summary

Closes out the top-50 PECL × arm64 matrix. Adds the 4 hard-tier extensions (`imagick`, `mongodb`, `swoole`, `grpc`) to linux/aarch64/nts — the arm64 parallel of B2 for x86_64.

Pure data slice. No Go code, no builder script, no workflow changes. All scaffolding is inherited from C1 (workflow runner map, per-arch phpup build, `(version, arch)` fixture filter, `installRuntimeDeps`) and C2a (arm64 ext build path + harness fixture shape).

Carry-over exclude from B2 (arch-independent, PECL metadata enforced):
- `swoole@8.1` — PECL `<min>8.2.0</min>`

Counts: 4 extensions × 5 PHP versions − 1 swoole@8.1 = **19 new arm64 ext bundles**. 19 existing hard-tier x86_64 cells also refresh (spec-hash churn from `abi_matrix.arch` change — content-identical rebuilds; same pattern every Phase-2 slice has exhibited).

Wall-clock ceiling ~45 min dominated by arm64 grpc × 5 (matrix-parallel) and x86_64 grpc × 5 refresh running concurrently.

After merge: `bundles.lock` at 182 entries (5 cores × 2 archs + 86 x86_64 ext + 86 arm64 ext minus the arch-specific carry-over excludes). Wait — recount: 96 entries after C1 + 67 from C2a = 163. +19 from C2b = **182**. Plus x86_64 `bundles.lock` entries stay unchanged (19 hard-tier x86_64 entries already there, only their spec_hashes refresh). `test/compat/fixtures.yaml` at 65 (+5 `multi-ext-hard4-arm64-<NN>`). Action release `v0.2.0-alpha.11`. Phase-2-exit release still waits on slice D (ubuntu-22.04).

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `ext:imagick:3.8.1:{8.1..8.5}:linux:aarch64:nts`, `ext:mongodb:2.2.1:{8.1..8.5}:linux:aarch64:nts`, `ext:swoole:6.2.0:{8.2..8.5}:linux:aarch64:nts`, `ext:grpc:1.80.0:{8.1..8.5}:linux:aarch64:nts` — 19 new entries.
- Each of the 4 `catalog/extensions/<name>.yaml` files has `abi_matrix.arch` expanded from `["x86_64"]` to `["x86_64", "aarch64"]`. Swoole's existing `exclude: [{ php: '8.1' }]` stays.
- 5 new `multi-ext-hard4-arm64-<NN>` fixtures, one per PHP version; 8.1 variant omits swoole.
- Harness passes 65 fixtures × 3 jobs = 195 cells with no new allowlist entries beyond existing bands.

### Non-goals
- ubuntu-22.04 (slice D).
- macOS / Windows (Phases 5 / 6).
- PECL version bumps for the 4 extensions. Single-purpose slices per convention.
- New infrastructure. Inherits 100% from C1 + C2a.
- Per-extension single-ext fixtures on arm64. Shared multi-ext-hard4-arm64 is the primary compat check; build-time smoke is the per-extension isolation check.

## 3. Source of truth

Same pinning as every Phase-2 slice since #1:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` (no new audit; versions pinned in B2 carry forward) |

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/extensions/imagick.yaml` | `abi_matrix.arch: ["x86_64"]` → `["x86_64", "aarch64"]`. |
| `catalog/extensions/mongodb.yaml` | Same. |
| `catalog/extensions/swoole.yaml` | Same. Existing `exclude: [{ php: "8.1" }]` stays — it's arch-independent. |
| `catalog/extensions/grpc.yaml` | Same. |
| `test/compat/fixtures.yaml` | 5 new `multi-ext-hard4-arm64-<NN>` per §4.2. |
| `cmd/phpup/main.go` | **Unchanged.** Runtime-catalog map already lists imagick/mongodb/swoole/grpc with arch-uniform `RuntimeDeps` from B2. |
| Everything else (`internal/*`, `cmd/planner`, `builders/linux/*`, `.github/workflows/*`) | **Unchanged.** |

### 4.2 Fixture shape

Mirrors B2's `multi-ext-hard4-<NN>` naming with `-arm64` suffix and `arch: 'aarch64'`:

```yaml
# 8.1 omits swoole (PECL metadata min PHP 8.2).
- name: multi-ext-hard4-arm64-81
  php-version: '8.1'
  arch: 'aarch64'
  extensions: 'imagick, mongodb, grpc'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-hard4-arm64-82
  php-version: '8.2'
  arch: 'aarch64'
  extensions: 'imagick, mongodb, swoole, grpc'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-hard4-arm64-83
  php-version: '8.3'
  arch: 'aarch64'
  extensions: 'imagick, mongodb, swoole, grpc'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-hard4-arm64-84
  php-version: '8.4'
  arch: 'aarch64'
  extensions: 'imagick, mongodb, swoole, grpc'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-hard4-arm64-85
  php-version: '8.5'
  arch: 'aarch64'
  extensions: 'imagick, mongodb, swoole, grpc'
  ini-values: ''
  coverage: 'none'
```

### 4.3 Planner matrix expectation

After the catalog change, planner emits 19 new arm64 ext cells (imagick × 5 + mongodb × 5 + swoole × 4 + grpc × 5) plus 19 x86_64 refresh cells (spec-hash churn). Existing 67 arm64 ext entries from C2a stay in `bundles.lock` and are skipped by the lockfile check (unchanged YAMLs → matching spec_hashes → skip).

### 4.4 Wall-clock

grpc-dominated. 5 × arm64 grpc + 5 × x86_64 grpc refresh = 10 grpc builds at ~30-45 min each, matrix-parallel (`max-parallel: 30` in `plan-and-build.yml`). Ceiling ~45 min wall-clock. Other 24 cells (imagick/mongodb/swoole × 5 each, minus swoole@8.1) finish in ~5-10 min each.

## 5. Testing

### 5.1 Unit tests

None new. Test surface unchanged from C1 + C2a.

### 5.2 Per-extension build-time smoke

Each of the 4 YAMLs has its existing `smoke: ['php -r "assert(extension_loaded(\"<name>\"));"']`. Runs at build time on the arm64 cell per PHP version. Catches ABI mismatch / missing runtime dep / wrong path in isolation.

### 5.3 Harness

5 new `multi-ext-hard4-arm64-<NN>` fixtures. Harness grows 180 → 195 matrix cells per CI run.

### 5.4 Cross-slice regression

All 60 existing fixtures (50 x86_64 + 5 bare-arm64 from C1 + 5 multi-ext-top14-arm64 from C2a) keep running. Automatic via fixture-matrix design.

### 5.5 Coverage

Per `CLAUDE.md`: 80% per-package. No new production code. Numbers held by construction.

## 6. Release

- Single PR, branch `phase2-aarch64-c2b`. Rebased onto origin/main before every push.
- Commits:
  - `feat(catalog): extend hard-tier PECL extensions to aarch64 (phase 2 C2b)`
  - `test(compat): add multi-ext-hard4-arm64 harness fixtures`
  - `chore(lock)` — bot-committed after pipeline.
- release-please → `v0.2.0-alpha.11`. Phase-2-exit `v0.2.0-alpha` still waits on slice D.
- Rollback: per-extension arm64 fail → `exclude: { arch: "aarch64" }` on that extension's YAML.

## 7. Open questions

Same playbook as B2 / C2a:

1. **imagick arm64 IM6 linkage** — Ubuntu noble ships `libmagickwand-6.q16-7t64` / `libmagickcore-6.q16-7t64` for both archs (arch-uniform main archive). Expected clean. If IM6 can't link on arm64 (unlikely), `exclude: { arch: "aarch64" }` + tracking issue.
2. **grpc arm64 bundled-source compile** — grpc upstream has first-class arm64 support since ≥1.50; PECL grpc 1.80 should compile cleanly. If bundled-source fails, fall back to system `libgrpc-dev`/`libprotobuf-dev` per B2 §7 open-Q 3.
3. **swoole configure on arm64** — default configure (no opt-in flags). swoole upstream supports arm64 natively since v5.0+. Expected clean.
4. **mongodb OpenSSL on arm64** — `libssl3t64`/`libsasl2-2` arch-uniform on noble. Expected clean.
5. **x86_64 spec-hash-refresh grpc cost** — 5 additional x86_64 grpc builds from the abi_matrix.arch change. Content-identical rebuilds but each runs ~30-45 min. Acceptable in context (one-time cost for the matrix expansion).
6. **Harness diff** — v2 installs these via `apt-get install php<ver>-<ext>` on both archs, so ini defaults should match. If drift surfaces, per-extension allowlist with `multi-ext-hard4-arm64*` wildcard.

## 8. References

- `docs/product-vision.md` §6.1, §12.2.
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4 (Phase-2 scope).
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 item 4 (aarch64 roadmap).
- `docs/superpowers/specs/2026-04-21-phase2-aarch64-c1-design.md` — aarch64 infrastructure + cores.
- `docs/superpowers/specs/2026-04-21-phase2-aarch64-c2a-design.md` — easy+medium PECL on arm64 (immediate predecessor).
- `docs/superpowers/specs/2026-04-21-phase2-hard4-ext-design.md` — B2 (x86_64 hard tier; this slice mirrors on arm64).
- `docs/compat-matrix.md` §2.4 (jit_aarch64 — unchanged since C1).
- `CLAUDE.md` — quality gates, commit style, coverage target.
