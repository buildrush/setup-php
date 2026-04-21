# Phase 2 — Top-10 PECL Extension Expansion (slice B1)

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-21
**Supersedes:** Nothing (follow-up slice from `2026-04-17-phase2-compat-slice-design.md` §8, item 3 "Extension matrix expansion to top-50")
**Target release:** `v0.2.0-alpha.7` on merge

## 1. Summary

Phase 2's umbrella expansion has already landed PHP version breadth (8.1–8.5 core + 4 PECL extensions × 5 versions via PRs #37, #39, #40, #41). This slice widens the PECL extension catalog from 4 (`redis`, `xdebug`, `pcov`, `apcu`) to 14, adding the 10 next-most-requested extensions that don't require bespoke build systems: `igbinary`, `msgpack`, `uuid`, `ssh2`, `yaml`, `memcached`, `amqp`, `event`, `rdkafka`, `protobuf`.

The harder tier (`imagick`, `mongodb`, `swoole`, `grpc`) is deferred to slice B2 because each has its own failure surface worth diagnosing in isolation — ImageMagick API drift, OpenSSL/SASL2 linkage, massive configure matrices, 30–45 min compile times. Packing all 14 into one PR would turn any single-extension failure into a long iteration cycle against the whole slice.

Each of the 10 extensions produces 5 bundles (one per PHP minor × linux/x86_64/nts). `bundles.lock` grows by 50 entries (total 74 after merge). `test/compat/fixtures.yaml` grows by 5 (one `multi-ext-top10-<NN>` per version). No new allowlist entries expected up front; per-extension ini drift against v2 is resolved in CI per the bundle-rollout PR-self-publish pattern.

This slice is the first to exercise the `runtime_deps.linux` catalog field (declared in slice #1, unused since). 5 of the 10 new extensions link against well-known Ubuntu-native libraries (`libmemcached`, `librabbitmq`, `libevent`, `librdkafka`, `libprotobuf`); the `phpup` runtime learns to `apt-get install` those before composing. This matches `shivammathur/setup-php@v2`'s behavior, keeping bundles small and compat promises stable. Bundling runtime libs inside the extension bundle (the product-vision §6.1 aspirational path) remains a future slice; the catalog field semantic is stable enough that upgrading doesn't break users.

## 2. Goals & non-goals

### Goals
- `bundles.lock` contains `ext:<name>:<pinned-version>:<phpMinor>:linux:x86_64:nts` entries for all 10 new extensions across 8.1–8.5, subject to per-extension `exclude:` entries if PECL compat audits surface gaps.
- `catalog/extensions/` contains one YAML per new extension, pinned to a specific PECL version, matching the shape of `catalog/extensions/redis.yaml` plus a new `build_deps.linux` field.
- `internal/catalog.ExtensionSpec` parses `build_deps` as `map[string][]string` (same shape as `runtime_deps`), with existing extensions (empty value) passing unchanged.
- `builders/linux/build-ext.sh` installs per-extension `build_deps.linux` (passed via `BUILD_DEPS` env from the workflow) before `phpize`. Empty `BUILD_DEPS` → no-op, existing 4 extensions' behavior unchanged.
- `cmd/phpup/main.go` installs per-extension `runtime_deps.linux` packages via `apt-get install -y -qq --no-install-recommends` after resolution, before extract. Linux-only; empty list → no-op; deduplicates across multiple extensions.
- `test/compat/fixtures.yaml` has one `multi-ext-top10-<NN>` fixture per PHP version loading all 10 new extensions plus any that are known to work.
- The compat-harness workflow passes all 45 existing fixture pairs (40 from slice A + 5 new) with no new allowlist entries beyond the existing `env_delta` / `extensions` / `path_additions` bands. Per-extension drift against v2 that does appear is resolved in CI per the PR-self-publish flow.

### Non-goals
- The 4 "hard" extensions (`imagick`, `mongodb`, `swoole`, `grpc`) — slice B2.
- Per-extension single-ext harness fixtures — deferred; one shared multi-ext fixture per version is the primary compat check, per-extension smoke tests remain the isolation check.
- Bundling shared libraries inside the extension bundle — future slice. This slice uses runtime apt-get, matching v2.
- Generating the `cmd/phpup` runtime catalog map from `catalog/extensions/` at build time. The hardcoded map (flagged as tech debt in PR #37) grows from 4 entries to 14 manually; the structural fix is its own slice.
- macOS / Windows — Phases 5 / 6.
- aarch64 — slice C of the Phase-2 umbrella.
- ubuntu-22.04 — slice D.
- `tools:` input support — Phase 3.
- Tier-2/3/4 fallback cascade — Phase 3.
- PECL version bumps on the existing 4 extensions (redis/xdebug/pcov/apcu) — single-purpose slices per spec convention. See issue #38 for redis 6.3.0.

## 3. Source of truth

Same pinning as slice #1 and all subsequent Phase-2 slices:

| Field | Value |
|---|---|
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |
| Audit date | `2026-04-17` (unchanged — no new audit is performed in this slice) |

Compat-matrix sections consulted:
- §4 "layers in scope" — L1/L2/L3 apply (action surface, semantic compat, default ini values).
- §5 — no disposition changes for this slice; allowlist may grow per in-PR CI evidence.

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `catalog/extensions/<name>.yaml` | 10 new files, one per extension. Shape: `kind: pecl`, `source.pecl_package: <name>`, pinned `versions: [<latest-stable-supporting-8.1-8.5>]`, `abi_matrix.{os: [linux], arch: [x86_64], ts: [nts], php: ["8.1","8.2","8.3","8.4","8.5"]}`, `build_deps.linux: [<apt-packages>]` (new field), `runtime_deps.linux: [<apt-packages>]`, `ini: ["extension=<name>"]`, `smoke: ['php -r "assert(extension_loaded(\"<name>\"));"']`. |
| `internal/catalog/catalog.go` | Add `BuildDeps map[string][]string` field to `ExtensionSpec` (tag `yaml:"build_deps,omitempty"`). Parallel to `RuntimeDeps`. Purely additive — old YAMLs parse with empty map. |
| `internal/catalog/catalog_test.go` | Table-driven test for `build_deps` parsing: present with linux packages, absent, empty-list. Mirrors existing `runtime_deps` test shape. |
| `builders/linux/build-ext.sh` | Before `phpize`, if `${BUILD_DEPS:-}` is non-empty, `$SUDO apt-get install -y -qq --no-install-recommends $BUILD_DEPS`. Empty → skip. |
| `.github/workflows/build-extension.yml` | New step before `build-ext.sh`: `BUILD_DEPS=$(yq eval '.build_deps.linux // [] | join(" ")' catalog/extensions/${{ inputs.extension }}.yaml)`; export to the builder's env. |
| `cmd/phpup/main.go` | Add `installRuntimeDeps(resolved []ResolvedBundle, cat *catalog.Catalog)` invoked after `resolve.Resolve()`, before `extract.ExtractParallel()`. Linux-only. Collects unique apt packages across `cat.Extensions[<name>].RuntimeDeps["linux"]`; empty → no-op. Runs `sudo apt-get update -qq && sudo apt-get install -y -qq --no-install-recommends <deduped-packages>`. |
| `cmd/phpup/main.go` (runtime catalog map) | Grow hardcoded `Extensions` map at `main.go:74-78` from 4 entries to 14. Each new entry lists `Kind: catalog.ExtensionKindPECL`, `Versions: [<pinned>]`, optional `Ini`. No other fields populated here. |
| `cmd/phpup/main_test.go` | `TestInstallRuntimeDeps` with mocked apt executor: empty input is no-op, duplicates collapse, non-linux skips. |
| `test/compat/fixtures.yaml` | 5 new fixtures: `multi-ext-top10-{81,82,83,84,85}`, each with `extensions: 'igbinary, msgpack, uuid, ssh2, yaml, memcached, amqp, event, rdkafka, protobuf'`, `coverage: 'none'`, empty `ini-values`. |
| `docs/compat-matrix.md` | No preemptive edit. Any drift surfaced by the harness in-PR gets a per-extension allowlist entry with documented justification (same pattern as existing xdebug/disable_functions entries). |
| `cmd/compat-diff`, `internal/compose`, `internal/resolve`, `internal/plan`, `cmd/planner`, `internal/oci`, `internal/extract`, `internal/cache`, `internal/env`, rest of `internal/` | **Unchanged.** |
| `.github/workflows/compat-harness.yml`, `plan-and-build.yml`, `on-push.yml`, all other workflows | **Unchanged.** |
| `README.md` | Not edited in this slice — per-PECL-extension listing drift is a doc-maintenance concern handled separately. |

### 4.2 Build-time dep install (chosen option)

**4.2-A — Per-extension `build_deps.linux` in YAML, installed by `build-ext.sh`.** Chosen. The catalog declares what an extension needs to compile; the builder applies the list before `phpize`. Additive, declarative, discoverable by reviewers via the catalog diff.

**4.2-B — Hard-code per-extension `apt-get install` in `build-ext.sh` via case statement.** Rejected: hides the dep list inside a script, scales badly, makes review harder.

**4.2-C — Per-extension builder script `builders/linux/build-ext-<name>.sh`.** Rejected: duplicates the 60 lines of common setup across 10 scripts; changes to the common flow (apt-get, fetch-core, phpize/configure/make) need N edits.

### 4.3 Runtime dep install (chosen option — v2 parity)

**4.3-A — `phpup` runs `apt-get install` for `runtime_deps.linux` before composing.** Chosen. Matches `shivammathur/setup-php@v2` behavior (which also apt-installs PECL runtime libs). Keeps bundle size small (<500 KB per extension), preserves install speed (apt is 2–5 s per package on a warm runner), and the catalog field semantic (`runtime_deps.linux` = "apt packages needed at runtime") is stable across the future upgrade to bundled libs.

**4.3-B — Bundle `.so` runtime libs inside the extension bundle.** Deferred. Matches product-vision §6.1 "plus any statically-linked runtime libraries" aspiration, but adds ~5-20 MB per bundle, new builder work (`ldd`-based dep extraction), and `LD_LIBRARY_PATH` plumbing that doesn't play cleanly with macOS/Windows plans. The catalog field stays the same, so we can upgrade later without breaking the action surface.

**4.3-C — Document the deps; require users to apt-install themselves.** Rejected: breaks the "drop-in replacement for v2" promise for extensions where v2 transparently installs the dep.

### 4.4 `installRuntimeDeps` behavior

Signature:
```go
func installRuntimeDeps(exts []resolve.ResolvedBundle, cat *catalog.Catalog, installer func([]string) error) error
```

Flow:
1. Short-circuit if `runtime.GOOS != "linux"`. Return nil.
2. Iterate `exts`; for each, look up `cat.Extensions[ext.Name].RuntimeDeps["linux"]`.
3. Collect into a set (deduplicate across extensions; e.g. multiple extensions might both want `libssl3`).
4. If set is empty, return nil.
5. Invoke `installer(sorted-slice)`. Default installer is a thin wrapper that `exec.Command("sudo", "apt-get", "update", "-qq")` then `exec.Command("sudo", "apt-get", "install", "-y", "-qq", "--no-install-recommends", pkg...)`.
6. Testing injects a mock installer; production uses the apt wrapper.

Error handling:
- apt-get update failure → `::warning::` (transient mirror issues shouldn't block); proceed to install.
- apt-get install failure → hard error, exits 1 with the apt stderr attached.
- `sudo` unavailable (shouldn't happen on GHA runners) → hard error with a clear "sudo required for runtime_deps" message.

### 4.5 Per-extension runtime-dep map

| Extension | `build_deps.linux` | `runtime_deps.linux` |
|---|---|---|
| `igbinary` | — | — |
| `msgpack` | — | — |
| `uuid` | `uuid-dev` | `libuuid1` (usually pre-installed) |
| `ssh2` | `libssh2-1-dev` | `libssh2-1` |
| `yaml` | `libyaml-dev` | `libyaml-0-2` |
| `memcached` | `libmemcached-dev libsasl2-dev zlib1g-dev` | `libmemcached11 libsasl2-2` |
| `amqp` | `librabbitmq-dev` | `librabbitmq4` |
| `event` | `libevent-dev libevent-openssl-dev` | `libevent-2.1-7 libevent-openssl-2.1-7` |
| `rdkafka` | `librdkafka-dev` | `librdkafka1` |
| `protobuf` | *(bundled mode — see §6 open Q1)* | *(bundled)* |

Versioned lib names (`libevent-2.1-7`, `libsasl2-2`, etc.) reflect ubuntu-24.04 (noble) package names. If slice D (ubuntu-22.04) lands later and the package names differ, `runtime_deps.linux` may need per-runner keying or equivalent — deferred.

### 4.6 Per-extension version pinning

Picked per the PECL metadata audit (performed in-PR, not in this spec). The decision rule:
1. Latest stable release that supports PHP 8.1–8.5 per its `package.xml` `<dependencies>/<php>` block.
2. If the latest stable excludes 8.1 or 8.5, pick the next-older stable that covers the full range.
3. If no single version covers 8.1–8.5, pin latest stable + add `exclude:` for the un-covered PHP versions (same pattern as redis 6.2.0 on 8.5 → issue #38).

This matches slice A's policy: version bumps are never hidden inside scope-expansion slices.

## 5. Testing

### 5.1 Unit tests

- `internal/catalog` — `build_deps` parsing table test (present linux packages, absent, empty array).
- `cmd/phpup` — `TestInstallRuntimeDeps` with mock installer: empty → no-op, dedup across multiple extensions, non-linux → no-op, installer error propagates.

### 5.2 Per-extension build-time smoke

Each new `<name>.yaml` has:
```yaml
smoke:
  - 'php -r "assert(extension_loaded(\"<name>\"));"'
```

Runs after `make install` in the builder. Catches ABI mismatch, missing runtime dep, wrong path, in isolation, per PHP version. This is the primary per-extension regression check.

### 5.3 Harness fixtures

5 new `multi-ext-top10-<NN>` fixtures, one per PHP version. Each loads all 10 extensions plus, implicitly, validates that the full set composes without collision, that ini defaults match v2, and that `apt-get`-installed runtime libs resolve.

Expected harness cost: 5 new fixtures × 3 jobs each (ours + theirs + diff) = 15 new matrix cells per CI run. Total harness workload grows from 40 × 3 = 120 cells to 45 × 3 = 135 cells.

### 5.4 Cross-slice regression

After the slice merges, existing slice-A fixtures (8.1–8.5 × 8 variants = 40) must stay green. Automatic via the fixture-matrix design.

### 5.5 Coverage

Per `CLAUDE.md`: 80% per-package. New code (`installRuntimeDeps`) ships with its test from day one. `internal/catalog` gains a table-test row for `build_deps`. Existing per-package coverage numbers must not regress.

### 5.6 Local development

`make bundle-ext EXT_NAME=memcached EXT_VERSION=<pinned> PHP_ABI=8.4-nts` continues to work — the builder reads `build_deps` from the catalog via the workflow, but local invocation via `make bundle-ext` passes an empty `BUILD_DEPS` by default; contributors testing extensions with system deps must either set `BUILD_DEPS=…` manually or rely on the Docker image having the deps pre-installed. A brief `CONTRIBUTING.md` note documents the convention.

## 6. Release

- **Single PR** for the whole slice per the bundle-rollout PR-self-publish invariant. 50 new ext bundles + any spec-hash refreshes on the 4 existing extensions (from adding the `build_deps` field to `ExtensionSpec` → the YAML parse layer changed, spec-hashes include the parsed struct form, so all 20 existing ext entries get their spec-hashes refreshed with content-identical digests).
- **Conventional commits**:
  - `feat(catalog): add top-10 PECL extensions (phase 2 B1)`
  - `feat(catalog): build_deps.linux for per-extension apt packages`
  - `feat(phpup): install runtime_deps for PECL bundles`
  - `feat(builders/linux): install build_deps from catalog`
  - `test(compat): add multi-ext-top10 fixtures`
  - `chore(lock)` — bot-committed after the PR pipeline completes.
- **release-please** — single patch alpha, `v0.2.0-alpha.7`. The Phase-2-exit `v0.2.0-alpha` (without dot-suffix) cuts after slice B2 (hard-tier extensions), C (aarch64), D (ubuntu-22.04) all land. This slice does not bump the alpha major.
- **Lockfile flow** — PR-α commit-back via `plan-and-build.yml`. The bot lockfile commit triggers a re-run of required workflows (ci-lint, integration-test, compat-harness) that historically sits in `action_required` until a maintainer clicks Approve; this is the GHA-enforced gate on bot-originated pushes and is the same approval flow every Phase-2 PR has gone through.
- **Rollback** — per-extension break: add `exclude:` on the affected PHP version or revert the extension's YAML addition. Slice-wide break: revert the lockfile commit and remove the 10 catalog YAMLs; GHCR digests become orphans and are reaped by `gc-bundles.yml` after `GC_MIN_AGE_DAYS` per bundle-rollout PR-β.2.
- **README** — untouched. The slice ships user-visible PECL coverage growth; doc update can ride a later maintenance PR once B2 is also in.

## 7. Open questions

Resolved per-PR, not upfront:

1. **`protobuf` bundled vs system libprotobuf.** PECL `protobuf` compiles with a bundled C++ libprotobuf by default. If the bundled build works cleanly across all 5 PHP versions, drop `libprotobuf-dev` from `build_deps.linux` and leave `runtime_deps.linux` empty. Decide in PR CI based on build-log evidence. If bundled mode fails on one or more versions, fall back to system lib.
2. **`ssh2` ABI on 8.5.** PECL `ssh2`'s metadata historically trails. If the latest stable doesn't list 8.5, add `exclude: { php: "8.5" }` and note as a follow-up (separate slice) — same pattern as redis@8.5 / issue #38.
3. **`event` vs noble's libevent.** If ubuntu-24.04's `libevent-dev` is newer than PECL `event` regression-tests against, either pin a different `event` version (its own slice) or `exclude:` the affected PHP version.
4. **Harness diff allowlist growth.** v2 installs these extensions via Ondrej PPA's apt packages (e.g. `php-memcached`); we build from PECL source. Per-extension ini drift (e.g. `memcached.sess_locking`, `amqp.channel_max`) may require per-extension `ignore`/`allow` entries in `docs/compat-matrix.md`. Each such entry needs a one-sentence reason documenting the v2 vs source-build behavior delta.
5. **`runtime_deps.linux` package naming per Ubuntu version.** Current design uses noble (ubuntu-24.04) names. Slice D adds ubuntu-22.04. If package names differ (e.g., `libevent-2.1-7` vs `libevent-2.1-6`), `runtime_deps` needs per-runner keying. Decide when slice D lands; stays out-of-scope here.
6. **`cmd/phpup` runtime catalog map growth.** 4 → 14 hardcoded entries. Still not auto-generated. Tech debt continues; structural fix is its own slice.
7. **Bundle-size impact.** 10 new extensions × 5 versions × <500 KB = ~25 MB additional GHCR storage. Negligible against the product-vision §13.5 ~2–3 GB steady-state target.

## 8. References

- `docs/product-vision.md` §6.1 (bundle kinds), §15 (long-tail strategy).
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` §4.4 (Phase-2 top-50 extension list).
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 item 3 (umbrella roadmap entry).
- `docs/superpowers/specs/2026-04-20-bundle-schema-and-rollout-design.md` (PR-self-publish invariant).
- `docs/superpowers/specs/2026-04-21-phase2-version-expansion-design.md` (slice A; this slice's immediate predecessor).
- `docs/compat-matrix.md` §1, §2, §5 (v2 baseline + deviations allowlist).
- `CLAUDE.md` (quality gates, commit style, coverage target).
- Issue #38 (redis 6.3.0 bump — representative of the "version-bump is its own slice" policy).
