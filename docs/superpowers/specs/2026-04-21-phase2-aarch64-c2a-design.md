# Phase 2 — Slice C2a: Easy+Medium PECL Extensions on aarch64

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (slice C2a of the Phase-2 umbrella; follow-up from C1 `2026-04-21-phase2-aarch64-c1-design.md`)
**Target release:** `v0.2.0-alpha.10` on merge

## 1. Summary

Slice C1 (PR #46, merged) landed the aarch64 infrastructure and 5 arm64 PHP cores. This slice enables the "easy+medium" tier of PECL extensions on arm64 — mirroring the B1/B2 split: `redis`, `xdebug`, `pcov`, `apcu` (slice-#1 four) + `igbinary`, `msgpack`, `uuid`, `ssh2`, `yaml`, `memcached`, `amqp`, `event`, `rdkafka`, `protobuf` (B1 ten). Hard tier (`imagick`, `mongodb`, `swoole`, `grpc`) deferred to C2b — each has its own known-failure surface worth isolating (grpc takes 30–45 min to build, wall-clock-dominating; imagick's IM6 linkage; swoole's configure matrix; mongodb's SSL). Splitting C2 the same way B was split gives symmetric blast radius: C2a is 67 cells of 5–10 min each, C2b will be 19 cells dominated by grpc.

Pure data slice — no Go code, no builder script, no workflow changes. 100% of the scaffolding is inherited from C1: workflow `runs-on` arch map, per-arch phpup build, `(version, arch)`-aware fixture filter, `installRuntimeDeps` reading arch-uniform `runtime_deps.linux` package names.

After merge: `bundles.lock` grows 96 → 163 (+67 arm64 ext: 14 × 5 − 3 existing excludes). `test/compat/fixtures.yaml` grows 55 → 60 (+5 `multi-ext-top14-arm64-<NN>`, one per PHP minor). Action release `v0.2.0-alpha.10`.

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `ext:<name>:<ver>:<minor>:linux:aarch64:nts` entries for 14 extensions × 5 PHP minors, minus the 3 existing upstream-source excludes that carry over arch-independently (redis@8.5, igbinary@8.5, protobuf@8.1). Net: 67 new entries.
- Each of the 14 `catalog/extensions/<name>.yaml` files has `abi_matrix.arch` expanded from `["x86_64"]` to `["x86_64", "aarch64"]`. No other changes to the YAMLs.
- `cmd/phpup/main.go`'s runtime catalog map is **unchanged** — `RuntimeDeps` package names are arch-uniform on Ubuntu noble and slice C1 verified `installRuntimeDeps` on arm64.
- 5 new `multi-ext-top14-arm64-<NN>` fixtures, one per PHP version; 8.1 variant omits `protobuf`, 8.5 variant omits `redis` + `igbinary`.
- Compat harness passes 60 fixtures × 3 jobs = 180 cells with no new allowlist entries beyond existing bands.

### Non-goals
- Hard-tier extensions (`imagick`, `mongodb`, `swoole`, `grpc`) — slice C2b.
- ubuntu-22.04 — slice D.
- macOS / Windows — Phases 5 / 6.
- PECL version bumps on any of the 14 extensions. Per-project convention: version bumps are their own single-purpose slices.
- New runtime-code paths, Go signature changes, or runtime-catalog-map rework. The C1 runtime is the final shape for aarch64 support in this slice.
- Expanded harness-fixture axes (per-coverage, per-ini-file) on arm64. The `multi-ext-top14-arm64-<NN>` multi-ext fixture plus the 5 `bare-arm64-<NN>` fixtures from C1 give sufficient smoke coverage for the data-level expansion.

## 3. Source of truth

Same pinning as every Phase-2 slice since #1:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` (no new audit; inherits C1 and B1) |

### 3.1 Carry-over excludes

Three existing excludes in `catalog/extensions/` apply arch-independently (no `arch:` field in the exclude, so they match both x86_64 and aarch64):

| Extension | Exclude | Reason | Tracking issue |
|---|---|---|---|
| `redis` | `{ php: "8.5" }` | `php_smart_string.h` removed in 8.5 (source-level incompat) | #38 |
| `igbinary` | `{ php: "8.5" }` | Same `php_smart_string.h` upstream issue | #43 |
| `protobuf` | `{ php: "8.1" }` | PECL metadata: `<min>8.2.0</min>` | — (built-in metadata constraint) |

These carry to arm64 verbatim. No new excludes added upfront for this slice; any arm64-specific compile failure surfaced in CI becomes `{ arch: "aarch64", php: "X.Y" }` per the standard playbook.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/extensions/redis.yaml` | `abi_matrix.arch: ["x86_64"]` → `["x86_64", "aarch64"]`. |
| `catalog/extensions/xdebug.yaml` | Same change. |
| `catalog/extensions/pcov.yaml` | Same change. |
| `catalog/extensions/apcu.yaml` | Same change. |
| `catalog/extensions/igbinary.yaml` | Same change. |
| `catalog/extensions/msgpack.yaml` | Same change. |
| `catalog/extensions/uuid.yaml` | Same change. |
| `catalog/extensions/ssh2.yaml` | Same change. |
| `catalog/extensions/yaml.yaml` | Same change. |
| `catalog/extensions/memcached.yaml` | Same change. |
| `catalog/extensions/amqp.yaml` | Same change. |
| `catalog/extensions/event.yaml` | Same change. |
| `catalog/extensions/rdkafka.yaml` | Same change. |
| `catalog/extensions/protobuf.yaml` | Same change. |
| `test/compat/fixtures.yaml` | 5 new `multi-ext-top14-arm64-<NN>` fixtures per §4.2. |
| `cmd/phpup/main.go` | **Unchanged.** Runtime catalog map already lists all 14 extensions with arch-uniform `RuntimeDeps`. |
| `internal/catalog`, `internal/resolve`, `internal/compose`, `internal/compat`, `cmd/planner`, `builders/linux/build-ext.sh`, `.github/workflows/*` | **Unchanged.** |
| `docs/compat-matrix.md` | No preemptive edits. Per-extension ini drift against v2 on arm64 (unlikely since ext ini defaults are arch-independent) → allowlist entries with documented justification. |
| `README.md` | Not edited. A later maintenance PR will bump the supported-matrix statement to "Linux x86_64 + aarch64" once C2b + D complete the arch story. |

### 4.2 Fixture shape (per PHP version)

Five new fixtures. The extension list per version accounts for the three carry-over excludes:

```yaml
- name: multi-ext-top14-arm64-81
  php-version: '8.1'
  arch: 'aarch64'
  extensions: 'redis, xdebug, pcov, apcu, igbinary, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-top14-arm64-82
  php-version: '8.2'
  arch: 'aarch64'
  extensions: 'redis, xdebug, pcov, apcu, igbinary, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka, protobuf'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-top14-arm64-83
  php-version: '8.3'
  arch: 'aarch64'
  extensions: 'redis, xdebug, pcov, apcu, igbinary, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka, protobuf'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-top14-arm64-84
  php-version: '8.4'
  arch: 'aarch64'
  extensions: 'redis, xdebug, pcov, apcu, igbinary, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka, protobuf'
  ini-values: ''
  coverage: 'none'

- name: multi-ext-top14-arm64-85
  php-version: '8.5'
  arch: 'aarch64'
  extensions: 'xdebug, pcov, apcu, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka, protobuf'
  ini-values: ''
  coverage: 'none'
```

8.1 omits `protobuf` (metadata min-PHP constraint). 8.5 omits `redis` and `igbinary` (php_smart_string.h upstream issue).

### 4.3 Planner matrix expectation

After the catalog change, the planner emits 67 new extension cells (14 × 5 − 3 excludes) all `arch: aarch64`. Existing x86_64 ext cells stay in `bundles.lock` and are skipped by the lockfile check.

Note: because `ComputeSpecHash` folds the full extension YAML into the spec-hash, changing `abi_matrix.arch` on an extension's YAML bumps the spec-hash of its **existing** x86_64 cell. All 14 x86_64 cells therefore also refresh (content-identical rebuilds, spec-hash update only) — same inefficiency flagged in PR #37 and seen on every prior Phase-2 slice. Not fixing in this slice; tracked as structural tech debt.

### 4.4 Runtime-catalog no-op

C1 verified that `installRuntimeDeps` works on `ubuntu-24.04-arm` and that Ubuntu's package set is arch-uniform (the 5 `bare-arm64-<NN>` fixtures included successful arm64 compose without new runtime-dep-install bugs). This slice introduces no new `RuntimeDeps` entries; the existing 14 extensions' `runtime_deps.linux` lists were populated in slice-#1 + B1 and tested on x86_64. The same package names resolve correctly on arm64 runners.

### 4.5 Lockfile growth shape

After C2a merges, `bundles.lock` grows from 96 to 163:

```
(existing 96 unchanged: 10 cores + 86 x86_64 ext)
ext:redis:6.2.0:{8.1,8.2,8.3,8.4}:linux:aarch64:nts               (4)
ext:xdebug:3.5.1:{8.1..8.5}:linux:aarch64:nts                     (5)
ext:pcov:1.0.12:{8.1..8.5}:linux:aarch64:nts                      (5)
ext:apcu:5.1.28:{8.1..8.5}:linux:aarch64:nts                      (5)
ext:igbinary:3.2.16:{8.1,8.2,8.3,8.4}:linux:aarch64:nts           (4)
ext:msgpack:3.0.0:{8.1..8.5}:linux:aarch64:nts                    (5)
ext:uuid:1.3.0:{8.1..8.5}:linux:aarch64:nts                       (5)
ext:ssh2:1.5.0:{8.1..8.5}:linux:aarch64:nts                       (5)
ext:yaml:2.3.0:{8.1..8.5}:linux:aarch64:nts                       (5)
ext:memcached:3.4.0:{8.1..8.5}:linux:aarch64:nts                  (5)
ext:amqp:2.2.0:{8.1..8.5}:linux:aarch64:nts                       (5)
ext:event:3.1.4:{8.1..8.5}:linux:aarch64:nts                      (5)
ext:rdkafka:6.0.5:{8.1..8.5}:linux:aarch64:nts                    (5)
ext:protobuf:5.34.1:{8.2,8.3,8.4,8.5}:linux:aarch64:nts           (4)
                                                              -----
                                                               67
```

## 5. Testing

### 5.1 Unit tests

No new unit tests. The scaffolding (`DefaultIniValues(phpVersion, arch)`, `BuildDeps`, `installRuntimeDeps`) is tested in C1 / B1 and continues to pass. Adding arch entries to `abi_matrix` is a data change; `internal/catalog` parsing tests already cover the shape.

### 5.2 Per-extension build-time smoke

Each extension's `smoke:` block (`php -r "assert(extension_loaded('<name>'));"`) runs at build time on the arm64 cell, catching ABI mismatch / missing runtime dep / wrong path per-extension per-version-per-arch. Primary per-extension regression check.

### 5.3 Harness fixtures

5 new `multi-ext-top14-arm64-<NN>` fixtures prove the full 14-extension set composes cleanly on arm64 for each PHP version. Harness cost grows 165 → 180 matrix cells per CI run.

### 5.4 Cross-slice regression

All existing 55 fixtures (50 x86_64 + 5 bare-arm64) continue to run; no coverage loss. Automatic via fixture-matrix design.

### 5.5 Coverage

Per `CLAUDE.md`: 80% per-package. This slice ships no new production code; per-package numbers are held by construction.

## 6. Release

- **Single PR, branch `phase2-aarch64-c2a`.** PR-self-publish per the bundle-rollout invariant. Rebased onto origin/main before every push.
- **Conventional commits:**
  - `feat(catalog): extend easy+medium PECL extensions to aarch64 (phase 2 C2a)`
  - `test(compat): add multi-ext-top14-arm64 harness fixtures`
  - `chore(lock)` — bot-committed after pipeline.
- **release-please** → `v0.2.0-alpha.10`. Phase-2-exit `v0.2.0-alpha` still waits on C2b + slice D.
- **Rollback** — per-extension arm64 failure: add `exclude: { arch: "aarch64", php: "X.Y" }` on the affected extension's YAML. Slice-wide rollback: revert the `abi_matrix.arch` expansion on all 14 YAMLs (14-line revert).

## 7. Open questions

Resolved in-PR per the standard playbook:

1. **Per-extension arm64 compile surprises.** The 14 extensions in scope are well-exercised on arm64 in the wider PHP ecosystem (each is in common use via `ppa:ondrej/php` which ships arm64 packages). Expected clean compile. Any failure → per-extension `exclude: { arch: "aarch64" }` + tracking issue.
2. **Spec-hash refresh churn on x86_64 cells.** 14 x86_64 cells rebuild content-identically (same `ComputeSpecHash`-includes-full-YAML issue seen on every slice). Wall-clock additive ~5–10 min matrix-parallel; tolerable.
3. **Harness diff drift.** v2 installs these extensions via Ondrej PPA's `apt-get install php<ver>-<ext>` path on both amd64 and arm64. Ini defaults are usually arch-independent (compiled-in defaults), so minimal drift expected. If per-extension ini drift appears on arm64 only, add a `multi-ext-top14-arm64*` allowlist entry (wildcard already supported in `compat-diff` since slice A).
4. **Bundle size on aarch64.** arm64 ext bundles should be similar-size to their amd64 equivalents (~100–500 KB). 67 new bundles × ~300 KB = ~20 MB additional GHCR storage. Negligible.
5. **CI minutes.** 67 real arm64 ext builds × ~5 min avg = ~335 min aggregate, matrix-parallel (`max-parallel: 30` in `plan-and-build.yml`) → ~11 min wall-clock for ext builds. Plus 14 x86_64 refresh builds. Plus harness. Total ~20–25 min. Well under the grpc-dominated wall-clock of C2b.

## 8. References

- `docs/product-vision.md` §6.1 (bundle kinds), §12.2 (multi-arch matrix).
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4 (Phase-2 umbrella).
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 item 4 (aarch64 roadmap).
- `docs/superpowers/specs/2026-04-21-phase2-aarch64-c1-design.md` — slice C1 (infrastructure + cores).
- `docs/superpowers/specs/2026-04-21-phase2-top10-ext-design.md` — slice B1 (easy+medium tier; this slice mirrors on arm64).
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` — PR-self-publish invariant.
- `docs/compat-matrix.md` §2.4 (jit_aarch64 audit, encoded by C1).
- `CLAUDE.md` — quality gates, commit style, coverage target.
- Issues #38 (redis@8.5), #43 (igbinary@8.5) — carry-over tracking.
