# Local + CI Unification Design

**Status:** Draft — 2026-04-23
**Scope:** Full pipeline (builds + tests) rework, phased over 6 PRs.

## Context

Today's CI/test topology has accumulated pain the authors explicitly want to eliminate:

- **Too many jobs per PR/push.** Dynamic matrices expand to 10–200+ jobs; the catalog-driven extension matrix is the main offender.
- **CI-only failure modes.** Jobs fail on issues that cannot be reproduced locally (runner-image preinstalls, GHCR timing, cross-job artifact handoff).
- **Chicken-and-egg artifacts.** `build-extension.yml` depends on `build-php-core.yml` having pushed to GHCR first; lockfile auto-commits back into the PR head ref mid-run. Feature branches are not self-contained.
- **Runner-image coupling.** Validation jobs run directly on `ubuntu-24.04` GitHub-hosted runners; builder scripts are careful, but workflow orchestration still assumes preinstalled Go/jq/yq/etc.
- **Local ≠ CI.** `make local-ci` exists, but it pulls already-published bundles from GHCR by digest — so it cannot validate the *current* branch's artifacts before GHCR has them. Local and CI diverge in orchestration (shell vs Actions YAML) and scope (2 fixtures vs 62).

**Goal.** Every build and test runs locally in docker with no external service dependencies, and CI runs the *same stack and mechanics* — only the `--registry` target differs. Matrix is flat: hard environment axes only (OS × ARCH × PHP version). All entry points are `make` targets used identically by developers and CI.

**Non-goal.** Unit tests (`go test ./...`) stay as they are. They run on a plain GitHub-hosted runner.

## Architecture

**One binary** (`phpup`) with subcommands. **One artifact format** (OCI layout on disk, same schema as GHCR blobs). **One test harness** (docker-wrapped matrix). **One CI workflow** (`ci.yml`, thin matrix).

```
┌────────────────────────────┐      ┌────────────────────────────┐
│  developer laptop          │      │  github actions runner     │
│  make ci                   │      │  ci.yml matrix             │
│    phpup test ──────┐      │      │    phpup test ──────┐      │
└─────────────────────┼──────┘      └─────────────────────┼──────┘
                      │                                   │
                      ▼                                   ▼
        ┌─────────────────────────────┐     ┌─────────────────────────────┐
        │ docker run ubuntu:22|24.04  │     │ docker run ubuntu:22|24.04  │
        │ (bare, hermetic, no apt)    │     │ (bare, hermetic, no apt)    │
        └────────────┬────────────────┘     └──────────────┬──────────────┘
                     │                                     │
                     ▼                                     ▼
        ┌──────────────────────────────────────────────────────────────┐
        │ registry (pluggable via --registry)                          │
        │   oci-layout:./out/oci-layout   ← local, hermetic (default)  │
        │   ghcr.io/buildrush             ← prod publish (CI main-only)│
        └──────────────────────────────────────────────────────────────┘

phpup build  writes →
phpup test   reads  ← (builds first on cache miss within the same cell)
phpup push   promotes oci-layout → ghcr.io (CI-only, on main, after pipeline passes)
```

**Key properties**

- The GitHub-hosted runner is just a docker host. No `apt-get install` in workflows, no reliance on runner preinstalls.
- Local and CI share the identical code path (`phpup test`); only `--registry` differs.
- **No cross-job GHCR roundtrip.** A pipeline cell builds PHP-core → builds extensions → runs fixtures against the just-built artifacts in *its own* OCI layout. GHCR is touched only by the `publish` job after the full matrix passes on `main`.
- Chicken-egg eliminated: nothing waits for a prior job's GHCR push.

## CLI surface

Single binary `cmd/phpup/` with subcommands. Maintainer-only subcommands live under `phpup internal <…>`.

| Subcommand | Replaces | Who runs it |
|---|---|---|
| `phpup install` (default) | today's `phpup` | end users (GitHub Action) |
| `phpup build php` | `builders/linux/build-php.sh` caller | maintainers + CI |
| `phpup build ext` | `builders/linux/build-ext.sh` caller | maintainers + CI |
| `phpup test` | `test/smoke/local-ci.sh` + `compat-harness.yml` orchestration | maintainers + CI |
| `phpup plan` | `cmd/planner` | CI |
| `phpup lockfile update` | `cmd/lockfile-update` | CI |
| `phpup push` | *new* — promote oci-layout → registry | CI (on main) |
| `phpup internal gc` | `cmd/gc-bundles` | cron |
| `phpup internal hermetic-audit` | `cmd/hermetic-audit` | CI cell |
| `phpup internal compat-diff` | `cmd/compat-diff` | maintainers |
| `phpup internal test-cell` | *new* — inner-container fixture runner spawned by `phpup test` | test-cell container only |

**Global flag:** `--registry <ref>` (env `INPUT_REGISTRY` / `PHPUP_REGISTRY`), accepted by every subcommand.
- `ghcr.io/buildrush` (default for `phpup push` and for `phpup install` when invoked as a published GitHub Action).
- `oci-layout:<path>` (default for `phpup build`, `phpup test`, and `make ci-cell`).

## Registry abstraction

New package `internal/registry/`:

```go
package registry

type Ref struct {
    Scheme string // "ghcr" | "oci-layout"
    Host   string // "ghcr.io/buildrush" or absolute filesystem path
    Name   string // "php-core", "php-ext-redis", ...
    Digest string // "sha256:…"
}

type Meta struct { /* mirrors existing meta.json */ }

type Store interface {
    Fetch(ctx context.Context, r Ref) (io.ReadCloser, *Meta, error)
    Push(ctx context.Context, r Ref, bundle io.Reader, meta *Meta) error
    Has(ctx context.Context, r Ref) (bool, error)
    Resolve(ctx context.Context, key string) (Ref, error) // lockfile lookup
}

func Open(uri string) (Store, error) // dispatch by scheme
```

Two backends:
- `internal/registry/remote` — wraps `go-containerregistry/pkg/v1/remote` (absorbs current `internal/oci/client.go`).
- `internal/registry/layout` — wraps `go-containerregistry/pkg/v1/layout` (OCI layout directory: `index.json` + `blobs/sha256/…`). No daemon, no network.

Every call site that today references `"ghcr.io/buildrush"` string-literally goes through `registry.Open(flag.Value)`. The existing `internal/oci` package becomes a thin adapter — or is deleted once call sites migrate.

Reused: `internal/lockfile` (keys → digests, no registry hostname stored), `internal/planner.spec_hash` (spec_hash is already digest-deriving input), `internal/catalog` (unchanged).

## `phpup build php|ext`

Wraps `builders/linux/*.sh` **unchanged**. The subcommand adds docker orchestration, spec-hash cache probe, OCI-layout write.

```
phpup build php --php 8.4 --os jammy --arch amd64 --ts nts \
  --registry oci-layout:./out/oci-layout \
  --cache ./.cache/phpup-build

Flow:
  1. spec_hash ← sha256(
         builders/linux/build-php.sh,
         builders/common/**,
         catalog/php-versions.yaml[php:8.4],
         os, arch, ts)
  2. registry.Has(Ref{Name:"php-core", Digest:<derived>}) → if true, print
     "cache hit" and exit 0.
  3. docker run --rm --platform linux/$arch \
       -v $PWD/builders:/builders:ro \
       -v $CACHE:/cache \
       -v $OUT:/out \
       ubuntu:$os \
       bash /builders/linux/build-php.sh --php 8.4 --ts nts
     (cross-arch via qemu-user-static when host ≠ target)
  4. Pack /out/bundle.tar.zst + /out/meta.json.
  5. registry.Push(...) → write into OCI layout (or GHCR).
```

`phpup build ext` uses the same shape, additionally fetching the prerequisite `php-core` from the same registry before invoking `build-ext.sh`. Because both builds write to the *same* local OCI layout within one process, there is no cross-job handoff.

## `phpup test`

Single matrix runner that replaces `test/smoke/local-ci.sh` *and* the orchestration inside `compat-harness.yml`. Fixtures stay in `test/compat/fixtures.yaml`; goldens in `test/compat/testdata/`.

```
phpup test \
  --registry oci-layout:./out/oci-layout \
  --os jammy,noble \
  --arch amd64,arm64 \
  --php 8.1,8.2,8.3,8.4,8.5 \
  --fixtures test/compat/fixtures.yaml \
  --cache ./.cache/phpup-test \
  [--parallel N]

For each (os, arch, php) cell:
  1. docker run --rm --platform linux/$arch \
       --network none \
       -v $PWD/out/oci-layout:/registry:ro \
       -v $PWD/phpup:/usr/local/bin/phpup:ro \
       ubuntu:$os \
       /usr/local/bin/phpup internal test-cell \
         --registry oci-layout:/registry \
         --fixtures-filter "os=$os,arch=$arch,php=$php"
  2. Inside the container: `phpup internal test-cell` iterates matching
     fixtures, runs `phpup install` per fixture, invokes
     test/compat/probe.sh, diffs against goldens.
  3. Report per-fixture pass/fail with structured output (JSON summary
     uploaded as artifact).
```

Notes:
- Bare `ubuntu:22.04` / `ubuntu:24.04` images — no preinstalled tools except what `phpup install` and the bundles themselves provide.
- `--network none` by default. A fixture that needs network (e.g., fetching a remote PHAR) opts in explicitly in `fixtures.yaml`.
- Cross-arch via `qemu-user-static` when host arch ≠ target; native runners preferred in CI (`ubuntu-24.04-arm` for arm64).

## CI workflow (`ci.yml`)

One workflow, three jobs plus publish. Runs on every PR and every push to `main`.

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-24.04
    steps: [checkout, setup-go, make lint]
  # gofmt, vet, golangci-lint, mod tidy, eslint, prettier

  unit:
    runs-on: ubuntu-24.04
    steps: [checkout, setup-go, make test]
  # go test -race -cover ./... (non-goal: unchanged)

  pipeline:
    needs: [lint, unit]
    strategy:
      fail-fast: false
      matrix:
        os:   [jammy, noble]
        arch: [amd64, arm64]
        php:  ["8.1", "8.2", "8.3", "8.4", "8.5"]
    runs-on: >-
      ${{ matrix.arch == 'arm64' && 'ubuntu-24.04-arm' || 'ubuntu-24.04' }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Restore cache
        uses: actions/cache@v4
        with:
          path: |
            out/oci-layout
            .cache/phpup-build
          key: phpup-${{ matrix.os }}-${{ matrix.arch }}-${{ matrix.php }}-${{ hashFiles('builders/**', 'catalog/**') }}
          restore-keys: |
            phpup-${{ matrix.os }}-${{ matrix.arch }}-${{ matrix.php }}-
      - run: make ci-cell OS=${{matrix.os}} ARCH=${{matrix.arch}} PHP=${{matrix.php}}
      - uses: actions/upload-artifact@v4
        if: github.ref == 'refs/heads/main'
        with:
          name: oci-layout-${{matrix.os}}-${{matrix.arch}}-${{matrix.php}}
          path: out/oci-layout

  publish:
    needs: pipeline
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/download-artifact@v4
        with:
          pattern: oci-layout-*
          path: out/merged-layout
          merge-multiple: true
      - run: phpup push --from oci-layout:./out/merged-layout --to ghcr.io/buildrush
      - run: phpup lockfile update
      - name: Commit lockfile
        run: |
          git config user.name "buildrush-bot"
          git add bundles.lock
          git diff --cached --quiet || git commit -m "chore: update bundles.lock"
          git push
```

**Job count per PR:** 1 (lint) + 1 (unit) + 20 (2 OS × 2 arch × 5 PHP) = **22**.
**Job count per `main` push:** 22 + 1 (publish) = **23**.

(Down from 10–200+ dynamic jobs today. The full validation matrix — including arm64 and cross-OS — runs on every PR; no deferral of defect detection to `main`. Only the GHCR `publish` job is `main`-only, because it's promotion, not validation.)

**Local equivalence — `make ci-cell` does exactly what CI does:**

```make
ci-cell:
	phpup build php  --os $(OS) --arch $(ARCH) --php $(PHP) \
	  --registry oci-layout:./out/oci-layout --cache ./.cache/phpup-build
	phpup build ext  --os $(OS) --arch $(ARCH) --php $(PHP) --all \
	  --registry oci-layout:./out/oci-layout --cache ./.cache/phpup-build
	# --all expands to every catalog extension whose manifest matches the
	# (os, arch, php) tuple. Individual --ext redis can be passed instead.
	phpup test       --os $(OS) --arch $(ARCH) --php $(PHP) \
	  --registry oci-layout:./out/oci-layout

ci:
	for os in jammy noble; do for arch in amd64 arm64; do for php in 8.1 8.2 8.3 8.4 8.5; do \
	  $(MAKE) ci-cell OS=$$os ARCH=$$arch PHP=$$php ; \
	done; done; done
```

## Caching

Two layers, both already aligned with how the codebase thinks about content-addressing.

**Layer 1 — OCI layout is the cache.** Content-addressed by design. Each bundle lives at `blobs/sha256/<digest>`. Before `phpup build` compiles anything, it computes `spec_hash` for the (builder scripts, common envs, catalog entry, os, arch, php, ts) tuple and probes `registry.Has(ref)`. Hit ⇒ skip compilation. This is the same mechanism today used to avoid redundant GHCR pushes; it's relocated behind the `registry.Store` interface.

**Layer 2 — GitHub Actions cache.** `actions/cache@v4` persists `out/oci-layout/` + `.cache/phpup-build/` across CI runs. Cache key: `phpup-${os}-${arch}-${php}-${hashFiles('builders/**','catalog/**')}` with loose prefix restore-keys. Hit ⇒ the layout already has the digest ⇒ Layer 1 short-circuits ⇒ cell spends its time only on fixture tests.

**Cache invalidation:**
- `builders/` touched → spec_hash changes for affected bundles.
- Catalog touched → spec_hash changes for affected entries only (the `hashFiles` key is coarse, busting Layer 2 for every cell, but Layer 1 inside each cell still dedupes every unaffected digest — so the cost is bounded to re-running the short-circuited builds, not rebuilding).
- Unchanged branches → full cache hit → cells run in a few seconds each (just the fixture probe).

## Critical files

**New:**
- `internal/registry/registry.go` — `Store` interface + `Open(uri)`.
- `internal/registry/remote/remote.go` — GHCR backend.
- `internal/registry/layout/layout.go` — OCI-layout backend.
- `cmd/phpup/build.go` — `phpup build php|ext` subcommand.
- `cmd/phpup/test.go` — `phpup test` subcommand + `phpup test-cell` inner loop.
- `cmd/phpup/push.go` — `phpup push` (oci-layout → ghcr.io promoter).
- `.github/workflows/ci.yml` — unified workflow.

**Modified:**
- `cmd/phpup/main.go` — subcommand dispatch, `--registry` flag, consolidation entry.
- `internal/oci/client.go` — in PR 1 refactored to delegate to `internal/registry`; deleted once all call sites migrate (by end of PR 3).
- `Makefile` — `ci`, `ci-cell`, update `bundle-php` / `bundle-ext` to call `phpup build`.

**Deleted (PR 5):**
- `.github/workflows/build-php-core.yml`
- `.github/workflows/build-extension.yml`
- `.github/workflows/plan.yml`, `plan-and-build.yml`, `on-push.yml`
- `.github/workflows/integration-test.yml`, `compat-harness.yml`
- `.github/workflows/nightly.yml`, `manual.yml`, `gc-bundles.yml`
- `test/smoke/local-ci.sh` (folded into `phpup test`)

**Retained:**
- `.github/workflows/ci-lint.yml` → folded into `ci.yml::lint`.
- `.github/workflows/watch-*.yml`, `security-rebuild.yml` (orthogonal; re-trigger `ci.yml`).
- `.github/workflows/release-please.yml`, `check-release-pr.yml` (release engineering).

**Deleted (PR 6):**
- `cmd/planner/`, `cmd/lockfile-update/`, `cmd/gc-bundles/`, `cmd/hermetic-audit/`, `cmd/compat-diff/` — all moved to `phpup` subcommands.

## Rollout — six PRs

Each PR is independently shippable and ends with CI green. No long-lived feature branch.

1. **Registry abstraction + `--registry` on `phpup install`.** Adds `internal/registry/` with both backends. `phpup install` accepts `--registry` (env fallback, default `ghcr.io/buildrush`). `internal/oci/client.go` delegates. No workflow changes.
2. **`phpup build php|ext` subcommand.** Docker-wraps existing shell builders. `make bundle-php` / `make bundle-ext` call `phpup build`. Existing `build-php-core.yml` / `build-extension.yml` internally switch to `phpup build` (same job shape, same GHCR push).
3. **`phpup test` subcommand + `make ci-cell`.** Orchestrates per-cell fixture run in bare-ubuntu containers. `test/smoke/local-ci.sh` becomes a thin wrapper (deleted in PR 5).
4. **New `ci.yml` in parallel with old workflows.** Both systems run on every PR/push for one week. `publish` job gated behind an env flag during the grace period. Both sides must pass to merge.
5. **Cut over.** Delete the old workflows and `test/smoke/local-ci.sh`. Flip `publish` to sole GHCR writer.
6. **Consolidate remaining CLI binaries.** Move `planner`, `lockfile-update`, `gc-bundles`, `hermetic-audit`, `compat-diff` under `phpup` subcommands. Delete the old `cmd/*` entrypoints.

**Rollback posture.** After PR 4 the old pipeline still works and can be re-enabled by reverting PR 5 alone. PRs 1–3 are additive; PR 6 is cosmetic.

## Verification

**Per-PR acceptance:**

- PR 1: unit tests for `internal/registry/{remote,layout}` round-trip Push/Fetch/Has against a `t.TempDir()` layout; against a local `distribution/distribution:3` spun up by the test. `phpup install --registry oci-layout:<dir>` succeeds end-to-end against a layout populated with a real bundle.
- PR 2: `make bundle-php` and `make bundle-ext` produce identical tarball contents (byte-for-byte or digest-equal) to the pre-change shell-only path. Cache probe short-circuits a second invocation.
- PR 3: `make ci-cell OS=jammy ARCH=amd64 PHP=8.4` runs to completion locally without network (after initial docker image pull) and produces green fixture output. Cross-arch `ARCH=arm64` runs under qemu.
- PR 4: new `ci.yml` passes alongside old workflows for at least one week (~7 days of merged PRs) with zero divergence in artifact digests.
- PR 5: `main`-branch push still publishes to GHCR; released bundles have identical digests to the pre-cutover run.
- PR 6: `phpup plan`, `phpup lockfile update`, etc., produce byte-identical output to the old binaries for the same inputs.

**End-to-end local verification (before merging PR 5):**

```bash
# Hermetic local reproduction of CI
make clean
make ci     # 20 cells, each a full build + test loop inside bare-ubuntu
# Exit 0 with no network access beyond initial docker image pull.
```

**End-to-end CI verification:**

- Open a draft PR on a feature branch; observe 22 jobs complete green.
- Merge to `main`; observe `publish` job run, GHCR artifacts updated, lockfile commit pushed.
- Release workflow (`release-please.yml`) unaffected by upstream changes.

## Open questions / future work

- **Matrix breadth expansion.** If PHP 8.6 lands, the matrix grows to 24 cells; still well inside the "small matrix" envelope. If additional OS variants are added (Debian, Alpine) the axes multiply — worth revisiting cell-sharding at that point.
- **Layer 2 cache granularity.** `hashFiles('builders/**','catalog/**')` is coarse. Layer 1 absorbs the waste for now; a finer key (per-entry spec_hash) can replace it if cache turnaround becomes a bottleneck.
- **Persistent local registry option.** If developers ask for it later, `Open("http://localhost:5000/buildrush")` already works (the `remote` backend handles any go-containerregistry ref); no design change needed.
