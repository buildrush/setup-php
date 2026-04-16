# `buildrush/setup-php` — Design Document

A greenfield reimplementation of `shivammathur/setup-php` that is faster, lower-maintenance, and reproducible, while remaining a drop-in replacement for existing workflows.

---

## Table of contents

1. [Context and motivation](#1-context-and-motivation)
2. [Analysis of the current action](#2-analysis-of-the-current-action)
3. [Requirements](#3-requirements)
4. [Core architectural pivot](#4-core-architectural-pivot)
5. [Runtime design — the action itself](#5-runtime-design--the-action-itself)
6. [Distribution layer — content-addressed bundles on GHCR](#6-distribution-layer--content-addressed-bundles-on-ghcr)
7. [Repository layout](#7-repository-layout)
8. [The catalog — bundles as data](#8-the-catalog--bundles-as-data)
9. [Build pipeline — when to build](#9-build-pipeline--when-to-build)
10. [Build pipeline — how to build](#10-build-pipeline--how-to-build)
11. [Content addressing and the lockfile](#11-content-addressing-and-the-lockfile)
12. [Concrete workflow files](#12-concrete-workflow-files)
13. [Matrix size and storage footprint](#13-matrix-size-and-storage-footprint)
14. [Bootstrap and ongoing operation](#14-bootstrap-and-ongoing-operation)
15. [Extension long-tail strategy](#15-extension-long-tail-strategy)
16. [Versioning and reproducibility](#16-versioning-and-reproducibility)
17. [Performance targets](#17-performance-targets)
18. [Migration path](#18-migration-path)
19. [Tradeoffs and risks](#19-tradeoffs-and-risks)
20. [Open questions](#20-open-questions)

---

## 1. Context and motivation

`shivammathur/setup-php` is the de facto standard GitHub Action for installing PHP in CI workflows. It is widely used, well-maintained, and extremely capable — supporting PHP 5.3 through 8.6 across Ubuntu, Windows, and macOS, dozens of extensions, and a catalog of PHP tools (Composer, PHPStan, Psalm, PHPUnit, and many more).

It is also **slow**, **brittle under load**, and **structurally hard to maintain**. Typical installs take 15–60 seconds on a warm runner; Ubuntu 24.04 runners in 2025 began reporting 2–3 minute install times for common configurations; and maintenance requires coordinated changes across approximately ten supporting repositories.

This document proposes a **ground-up reimplementation** that preserves the public API surface (so it remains a drop-in replacement) but replaces the internals with a content-addressed, prebuilt-bundle distribution model. The new action, tentatively named `buildrush/setup-php`, aims to reduce typical install times by 5–10×, eliminate runtime package-manager orchestration, and fold all maintenance into a single repository with declarative build specifications.

---

## 2. Analysis of the current action

### 2.1 What `shivammathur/setup-php` does

It is a TypeScript-based GitHub Action (~40% TypeScript, ~38% Shell, ~20% PowerShell, ~1% PHP) that orchestrates OS-specific package managers to install PHP and related tooling:

- **Linux**: installs from `ppa:ondrej/php` via `apt-get`.
- **macOS**: installs from `shivammathur/homebrew-php` and `shivammathur/homebrew-extensions` via Homebrew.
- **Windows**: installs via `mlocati/powershell-phpmanager` pulling DLL binaries from PECL.
- **Extensions** come from apt packages, Homebrew formulas, PECL (sometimes compiled at runtime), or arbitrary git repositories.
- **Tools** are installed via composer global install, phive, or direct phar downloads.

### 2.2 Why it is slow

Five compounding tax sources operate on every run:

1. **`apt-get update` on the hot path.** Every Linux run talks to Launchpad or its mirrors, refreshes package indexes, and resolves dependencies. Mirror variance alone can add 30+ seconds.
2. **Dependency reconciliation.** On Ubuntu 24.04, the pre-installed PHP is from Ubuntu's own repositories, not ondrej's PPA. Adding the PPA triggers a cascade of dependency upgrades rather than a clean install — this is the root cause of the "20 seconds → 2–3 minutes" regression reported in discussion #886 of the upstream repository.
3. **Sequential, non-atomic orchestration.** Extensions install one at a time. If step 7 of 10 fails, the environment is left half-configured.
4. **Runtime compilation of some extensions.** `intl` with a specific ICU version, `event`, `gearman`, and custom PECL versions compile from source on the runner, burning 60–120 seconds of CPU each.
5. **Node.js bootstrap plus many child processes.** TypeScript means ~300 ms of Node cold-start, then dozens of subprocess spawns into bash or PowerShell, each with its own startup cost.

### 2.3 Why it is high-maintenance

- **Ten supporting repositories** must be kept in sync: `homebrew-php`, `homebrew-extensions`, `php-builder`, `php-builder-windows`, `icu-intl`, `php-ubuntu`, `php5-darwin`, `php5-ubuntu`, `composer-cache`, `cache-extensions`, and more.
- **Three languages** in the main repo (TypeScript, Bash, PowerShell), each with its own idioms and tests.
- **Upstream dependencies are out of the maintainer's control** — when Ondřej Surý publishes a breaking package update, users' workflows break until a patch lands.
- **The committed `dist/` bundle** must be updated on every code change, a well-known anti-pattern for JavaScript actions.
- **Adding a new extension** can touch TS code, shell scripts, PowerShell scripts, Homebrew formulas, and multiple documentation files — hours of review per PR.

### 2.4 Why it is unreliable in edge cases

- Upstream PPA or Homebrew outages propagate directly to user workflows.
- PECL's website has historical downtime.
- Compiled-on-runner extensions produce different binaries on different runner images — identical inputs can yield different outcomes over time.
- There is no per-bundle signature or provenance — users have to trust whatever the package managers serve that day.

---

## 3. Requirements

### 3.1 Functional

- **Drop-in compatibility.** Any workflow currently using `shivammathur/setup-php@v2` must work by changing only the `uses:` line to `buildrush/setup-php@v1`. All inputs (`php-version`, `extensions`, `ini-values`, `coverage`, `tools`, etc.), all environment flags (`phpts`, `update`, `fail-fast`), all outputs (`php-version`), and all documented behaviors must be preserved.
- **Same OS/arch coverage.** Ubuntu 22.04/24.04 (x86_64 and aarch64), Windows Server 2022/2025, macOS 14/15/26 (arm64 and x86_64).
- **Same PHP version range.** 5.6 through 8.6 (nightly), per-OS as currently supported.
- **Same extension and tool catalog**, at least for the top 50 extensions and all currently-listed tools.

### 3.2 Non-functional

- **Faster.** Target 5–10× reduction in typical install time.
- **Reproducible.** A pinned action version produces a byte-identical install a year later, assuming the registry is still online.
- **Low maintenance.** Adding an extension should be a one-file YAML PR. No cross-repo coordination.
- **Reliable.** Decouple user workflow success from third-party package-manager uptime.
- **Good developer experience.** Clear error messages, dry-run mode, first-class local parity.

### 3.3 Operational constraints

- **Single dedicated GitHub repository** (`buildrush/setup-php`).
- **Prebuilt binaries and dependencies stored on GitHub Container Registry** (GHCR) as OCI artifacts.
- **All build pipelines implemented as GitHub Actions workflows** in the same repository. No external build infrastructure.

---

## 4. Core architectural pivot

> **Stop installing PHP at runtime. Start materializing a prebuilt, content-addressed filesystem tree.**

Package managers are the wrong primitive for CI. CI environments are ephemeral and deterministic — they do not need a dependency solver, rollback semantics, or the ability to coexist with other running software. What they need is: *drop a known-good directory tree into place, prepend it to `PATH`, exit.*

Every `(OS, architecture, PHP minor version, thread-safety, extension, extension version)` tuple is a constant. These constants can be built **once, offline, in a maintainer-controlled pipeline**, and shipped as immutable, signed OCI artifacts. The action's job at runtime reduces to: identify which artifacts are needed, pull them in parallel, extract, symlink, exit.

This inverts the current design. Today the action does the hard work on every consumer run. In the new design the hard work is done once per (spec, builder script, base image) triple, in the maintainer's CI, and amortized across every consumer run thereafter.

---

## 5. Runtime design — the action itself

### 5.1 Language and packaging

The runtime is a **single Go binary**, cross-compiled per runner OS and architecture. Not TypeScript. Rationale:

| Concern                    | Current (TypeScript)                         | Proposed (Go)                        |
| -------------------------- | -------------------------------------------- | ------------------------------------ |
| Cold start                 | ~300 ms Node bootstrap + ~100 ms overhead    | ~10 ms                               |
| Parallel I/O               | `Promise.all` over `child_process.exec`      | Real goroutines, connection pooling  |
| Ship size                  | Committed `dist/` (~2 MB minified JS)        | Fetched lazily, not in repository    |
| Cross-OS story             | Three codebases (TS + bash + PowerShell)     | One codebase, three compile targets  |
| Dependency surface         | 200+ transitive npm packages                 | ~10 vetted Go modules                |
| Repository hygiene         | Must commit minified JS                      | Clean source tree                    |

Rust is equally viable; Go is chosen for fast cross-compile, mature OCI client libraries, small static binaries, and fit with the broader GitHub tooling ecosystem.

### 5.2 Action surface

`action.yml` remains structurally identical to `shivammathur/setup-php`'s. It declares the same inputs and outputs. Under the hood it uses a minimal Node.js bootstrap (GitHub Actions' JavaScript action format) that execs the correct platform-specific Go binary. The binary itself is fetched once per action version and cached by the runner.

### 5.3 Runtime flow

1. **Parse inputs.** Convert action inputs into a canonical `Plan` struct: PHP version, extensions, ini values, coverage driver, tools, flags.
2. **Resolve the plan against the embedded lockfile.** The binary ships with `bundles.lock` compiled in; every resolvable (PHP version, extension, tool) maps to an exact OCI digest. No network calls are needed for resolution.
3. **Probe the tool cache.** Check `$RUNNER_TOOL_CACHE/buildrush/<plan-hash>`. If present and valid, symlink into `/opt/buildrush/php-current`, export `PATH`, exit. Expected: 1–2 seconds.
4. **Miss path.** Fetch all required OCI artifacts in parallel over HTTP/2 from GHCR, with an R2/Cloudflare fallback mirror.
5. **Verify.** Check each blob's SHA-256 against the plan's expected digest. Verify cosign signatures.
6. **Extract.** Parallel zstd decompression; each bundle into its own directory under `/opt/buildrush/bundles/`.
7. **Compose.** Symlink extension `.so`/`.dll` files into the core PHP's extensions directory. Write ini fragments to `conf.d/`. Symlink tools into `/opt/buildrush/bin/`.
8. **Export environment.** Write `PATH`, `PHP_INI_SCAN_DIR`, and the `php-version` output via `$GITHUB_ENV` and `$GITHUB_OUTPUT`.
9. **Seed the warm cache.** Persist the composed tree under `$RUNNER_TOOL_CACHE/buildrush/<plan-hash>` for the next run.

### 5.4 New developer-experience features

Added without breaking the existing surface:

- **`mode`** input: `fast` (fail if no prebuilt bundle), `hermetic` (ignore pre-installed PHP, guarantee reproducibility), `compat` (mirror current shivammathur behavior). Default: `fast` falling back to `compat`.
- **`dry-run: true`**: prints the resolved bundle plan without downloading. Useful for debugging.
- **`cache-key` output**: a stable hash of the resolved plan, usable as a cache key for downstream steps.
- **`phpup doctor` subcommand**: prints resolved bundles, tool-cache state, disk usage, and suggestions.
- **Local parity**: the same binary installed on a developer laptop reproduces the exact same tree as CI. Closes the "works on my machine" gap.

---

## 6. Distribution layer — content-addressed bundles on GHCR

### 6.1 What a bundle is

A **bundle** is a zstd-compressed tarball containing a hermetic install of one logical unit — either a PHP core, a single extension, or a single tool. It is accompanied by a JSON metadata sidecar. Bundles are pushed to GHCR as generic OCI artifacts (using `oras`), not as container images.

Three bundle kinds:

- **`php-core`**: `bin/php`, `bin/php-cgi`, `bin/phpize`, `bin/php-config`, `bin/pecl`, `bin/pear`, the stock bundled extensions for that build, `lib/libphp*`, ini templates. Typical size: 15–20 MB zstd.
- **`php-ext`**: a single `.so` (Linux/macOS) or `.dll` (Windows), plus any statically-linked runtime libraries that could not be bundled into the binary itself. Typical size: 50 KB – 5 MB.
- **`php-tool`**: a phar or native binary for composer, phpstan, psalm, phpunit, etc. Architecture-independent for phars; per-arch for native. Typical size: 100 KB – 10 MB.

### 6.2 Bundle identity

Every bundle is identified by a content-addressed SHA-256 digest, computed over:

```
digest = sha256(
    canonical_yaml(expanded_catalog_cell)
  || builder_script_hash
  || base_runner_image_digest
  || builder_tool_versions        # compiler, linker, autoconf versions
)
```

Bundles are published to `ghcr.io/buildrush/php-{kind}-{name}@sha256:<digest>`. The tag naming scheme is:

```
ghcr.io/buildrush/php-core:8.5.0-linux-x86_64-nts
ghcr.io/buildrush/php-ext-redis:6.2.0-8.5-nts-linux-x86_64
ghcr.io/buildrush/php-tool-phpstan:1.11.0
```

Tags are convenience pointers; digests are canonical. Consumers always resolve to a digest before pulling.

### 6.3 Why GHCR

- **No anonymous pull rate limit** for public packages.
- **CDN-backed**, globally replicated.
- **Free** for public packages.
- **OCI-standard**, works with `oras` for generic artifact upload.
- **Integrated with GitHub Actions** via `GITHUB_TOKEN` — no separate secret management.
- **Supports cosign signatures** with keyless OIDC-backed signing.

A Cloudflare R2 mirror is added as a fallback (the setup-php project already has Cloudflare sponsorship), selected at runtime if GHCR returns a transient error.

### 6.4 Signatures and provenance

Every published bundle is signed with `cosign` using GitHub OIDC (no long-lived keys). An SBOM is generated at build time and attached as an OCI reference. The runtime binary verifies the signature before extraction; signature failures are fatal unless `--unsafe-skip-verify` is passed (not recommended, never the default).

---

## 7. Repository layout

Single repository. The action and the bundle pipeline live together because they co-version: a PR that adds an extension spec is the same PR that produces bundles and updates the lockfile.

```
buildrush/setup-php/
├── action.yml                      # public action surface
├── cmd/
│   ├── phpup/                      # Go source for the runtime binary
│   └── planner/                    # Go source for the build planner
├── internal/
│   ├── plan/                       # input parsing, plan resolution
│   ├── oci/                        # OCI client, digest verification
│   ├── extract/                    # zstd extraction, symlinking
│   └── ini/                        # ini composition
├── catalog/
│   ├── php.yaml                    # PHP core build recipes
│   ├── extensions/                 # one file per extension
│   │   ├── redis.yaml
│   │   ├── intl.yaml
│   │   ├── xdebug.yaml
│   │   └── ...
│   └── tools/                      # phar / composer-based tools
│       ├── phpstan.yaml
│       ├── psalm.yaml
│       └── ...
├── builders/
│   ├── common/                     # shared helpers (pack-bundle, fetch-core)
│   ├── linux/
│   │   ├── build-php.sh
│   │   └── build-ext.sh
│   ├── macos/
│   │   ├── build-php.sh
│   │   └── build-ext.sh
│   └── windows/
│       ├── build-php.ps1
│       └── build-ext.ps1
├── bundles.lock                    # canonical state: spec-hash → bundle digest
├── .github/workflows/
│   ├── plan.yml                    # diff catalog → emit build matrix
│   ├── build-php-core.yml          # reusable: builds one PHP core
│   ├── build-extension.yml         # reusable: builds one extension
│   ├── build-tool.yml              # reusable: builds one tool
│   ├── on-push.yml                 # catalog-change triggered builds
│   ├── nightly.yml                 # scheduled full reconciliation
│   ├── watch-php-releases.yml      # polls php.net, fires dispatch
│   ├── watch-runner-images.yml     # polls GitHub runner images
│   ├── manual.yml                  # workflow_dispatch entrypoint
│   ├── security-rebuild.yml        # repository_dispatch for CVEs
│   ├── release-action.yml          # tags the action itself
│   └── gc-bundles.yml              # retention cleanup
└── test/
    ├── smoke/                      # per-bundle smoke tests
    └── integration/                # action-level end-to-end
```

The split between `catalog/` (declarative: *what exists*) and `builders/` (imperative: *how to build*) is deliberate. The overwhelming majority of contributions touch only `catalog/`.

---

## 8. The catalog — bundles as data

Every bundle build is fully described by one YAML file. The schema is strict and CI-validated.

### 8.1 Extension spec example

```yaml
# catalog/extensions/redis.yaml
name: redis
kind: pecl
source:
  pecl_package: redis
versions:
  - "6.2.0"
  - "6.1.0"
  - "5.3.7"
abi_matrix:
  php:   ["7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"]
  os:    ["linux", "macos", "windows"]
  arch:  ["x86_64", "aarch64"]
  ts:    ["nts"]                   # Windows adds "zts"
exclude:
  - { os: windows, arch: aarch64 } # not supported upstream
  - { php: "5.x" }
runtime_deps:
  linux:   []                      # pure C, no external libraries
  macos:   []
  windows: []
ini:
  - "extension=redis"
smoke:
  - 'php -r "assert(extension_loaded(\"redis\"));"'
```

### 8.2 PHP core spec

Similar shape, with `kind: php` and richer fields for configure flags, bundled extensions, and per-OS patches. PHP versions are pinned to specific point releases in the spec; `latest` resolution happens in the planner, not at action runtime.

### 8.3 Tool spec

```yaml
# catalog/tools/phpstan.yaml
name: phpstan
kind: phar
source:
  github_release: phpstan/phpstan
  asset: phpstan.phar
versions:
  - "1.11.0"
  - "1.10.67"
  - "latest-1.x"                   # resolved nightly
arch_independent: true
smoke:
  - 'phpstan --version'
```

### 8.4 Expansion and validation

The planner expands `abi_matrix` into concrete cells, filters by `exclude`, and computes a **spec hash** per cell. CI enforces that every catalog file is syntactically valid, semantically complete, and references only known base versions.

---

## 9. Build pipeline — when to build

Six trigger kinds feed the pipeline. Separating them keeps each workflow simple.

### 9.1 Trigger taxonomy

| Trigger                         | Frequency       | Typical work                                  |
| ------------------------------- | --------------- | --------------------------------------------- |
| 1. Push to `catalog/`           | Per PR / merge  | 10–200 bundles, 5–20 minutes                  |
| 2. Scheduled nightly            | Once per day    | Usually zero work; occasionally a refresh     |
| 3. New PHP release              | ~monthly        | ~50 ext × 6 OS/arch = ~300 bundles            |
| 4. Runner image update          | Few times/year  | PHP cores and statically-linked extensions    |
| 5. Manual `workflow_dispatch`   | As needed       | Whatever the maintainer specifies             |
| 6. Security advisory dispatch   | Rare, urgent    | Forced rebuild + gated promotion              |

### 9.2 Per-trigger behavior

**1. Push to `catalog/`** — the common case. The planner diffs `catalog/` against `bundles.lock`, computes affected matrix cells, and builds only those. Runs on PRs (for validation) and on `main` push (for publish).

**2. Scheduled nightly reconciliation** — `cron: "0 3 * * *"`. For each catalog entry with an unpinned version (`latest-6.x`, etc.), resolve against upstream today and check whether the resulting digest is already published. 95% of nightly runs do zero work and complete in under a minute.

**3. New PHP release** — a lightweight hourly cron polls `https://www.php.net/releases/active` and the nightly branch. When the JSON changes, it fires `repository_dispatch` with the new version. This triggers PHP core builds across the full OS/arch matrix, followed by extension rebuilds for the new ABI.

**4. Runner image update** — daily poll of the `actions/runner-images` changelog. When glibc (Linux) or VC++ runtime (Windows) bumps, rebuild PHP cores and any extensions that statically link against the changed library.

**5. `workflow_dispatch`** — manual entrypoint with inputs (`extension: redis`, `php_versions: "8.3,8.4"`, `force: true`). For debugging and emergency refreshes.

**6. Security advisory `repository_dispatch`** — external webhook with signed payload identifying a CVE and the affected artifact. Triggers forced rebuild even when the digest is unchanged; promotion is gated on maintainer review.

### 9.3 Trigger flow diagram

```
┌──────────────────────┐
│ Push to catalog/     │─┐
├──────────────────────┤ │
│ Scheduled nightly    │─┤
├──────────────────────┤ │    ┌─────────────┐    ┌──────────────────┐
│ New PHP release      │─┼───►│  plan.yml   │───►│ skip-if-present? │
├──────────────────────┤ │    │ catalog →   │    │ check GHCR by    │
│ Runner image update  │─┤    │ matrix JSON │    │ content digest   │
├──────────────────────┤ │    └─────────────┘    └──────────────────┘
│ workflow_dispatch    │─┤                                │
├──────────────────────┤ │                                ▼
│ Security advisory    │─┘                ┌──────────────────────────┐
└──────────────────────┘                  │  build-php-core.yml      │
                                          │  build-extension.yml     │
                                          │  build-tool.yml          │
                                          │  (reusable workflows)    │
                                          └──────────────────────────┘
                                                        │
                                                        ▼
                                          ┌──────────────────────────┐
                                          │  Smoke + cosign sign     │
                                          └──────────────────────────┘
                                                        │
                                                        ▼
                                          ┌──────────────────────────┐
                                          │  Push to GHCR (immutable)│
                                          └──────────────────────────┘
                                                        │
                                                        ▼
                                          ┌──────────────────────────┐
                                          │  Auto-PR: update lock    │
                                          └──────────────────────────┘
```

---

## 10. Build pipeline — how to build

Every trigger feeds the same four-stage pipeline.

### 10.1 Stage A — Plan

Reads `catalog/` and `bundles.lock`, emits three matrix JSON files (one each for PHP cores, extensions, and tools). Runs on one small Ubuntu runner in ~20 seconds. Output matrices are consumed by downstream reusable workflows.

### 10.2 Stage B — Build

A reusable workflow per bundle kind is called with the matrix produced by Stage A. Each matrix cell is one isolated job. `fail-fast: false` so cells fail independently without canceling siblings. `max-parallel` throttles concurrent jobs to a realistic number given GitHub's per-account concurrent-job budget.

Per-kind workflows:

- **`build-php-core.yml`**: compiles PHP from source on the target runner OS using the recipe in `catalog/php.yaml`. Configure flags, bundled extensions, and patches are controlled by the catalog entry.
- **`build-extension.yml`**: fetches the prerequisite PHP core bundle (already on GHCR), uses its `phpize` to build the extension from PECL or git, packs the `.so`/`.dll` plus runtime libraries.
- **`build-tool.yml`**: downloads the phar or compiles the native binary, verifies upstream signatures where available, packs it with an `ini` directive if needed.

### 10.3 Stage C — Verify

For every successfully-built bundle: in a fresh runner matching the target OS, pull the just-pushed bundle, extract it, run the `smoke:` commands from the catalog entry. This catches "built fine, does not load" failures (ABI mismatch, missing runtime library, wrong path) immediately.

### 10.4 Stage D — Promote

Only after all bundles in the plan verify. Opens an automated PR updating `bundles.lock` with the new digests. Once the PR merges, a separate workflow advances the mutable `:stable-<abi>` tags on GHCR to point at the new digests.

Key invariant: **bundles are immutable by digest, but the `:stable-*` tags and the `bundles.lock` entries can roll forward or back**. A bad release is rolled back by reverting the lockfile PR and advancing the tags in the other direction — no artifact deletion, no mutation.

---

## 11. Content addressing and the lockfile

### 11.1 The `bundles.lock` file

A flat JSON file, auto-generated and PR-merged, mapping canonical spec keys to OCI digests:

```json
{
  "schema_version": 1,
  "generated_at": "2026-04-16T03:00:00Z",
  "bundles": {
    "php:8.5.0:linux:x86_64:nts":               "sha256:3a1f...",
    "php:8.5.0:linux:aarch64:nts":              "sha256:c9e2...",
    "php:8.5.0:windows:x86_64:nts":             "sha256:41ab...",
    "php:8.5.0:windows:x86_64:zts":             "sha256:8dc7...",
    "ext:redis:6.2.0:8.5:linux:x86_64:nts":     "sha256:7bd4...",
    "ext:intl:bundled:8.5:linux:x86_64:nts":    "sha256:f001...",
    "tool:composer:2.7.7:any:any":              "sha256:9e3e...",
    "tool:phpstan:1.11.0:any:any":              "sha256:b2aa..."
  }
}
```

### 11.2 How it is used

- **By the planner**: before building a matrix cell, check whether its digest already exists in the lockfile AND in GHCR. If yes, skip. This makes most pipeline runs no-ops.
- **By the action binary**: the lockfile is compiled into the Go binary at action release time. At runtime, resolving a user's request to a set of OCI digests is a pure in-memory lookup, zero network calls.
- **By users**: because the lockfile is embedded in the binary and the binary is pinned to an action version, pinning `buildrush/setup-php@v1.4.2` guarantees byte-identical installs across time, as long as GHCR still serves the referenced digests.

### 11.3 Properties of content addressing

- **Idempotence**: retrying a failed build job does not duplicate artifacts; the push is a no-op if the digest already exists.
- **Deduplication**: an upstream mirror pull (e.g. PECL) that produces byte-identical output does not republish.
- **Audit**: every published bundle has exactly one provenance — the build job whose output hashed to that digest.
- **Rollback**: reverting the lockfile PR is a full rollback, because the old digests still exist on GHCR (they are never deleted by promotion).

---

## 12. Concrete workflow files

Full, runnable-shape examples. Trimmed for readability but structurally complete.

### 12.1 `plan.yml`

```yaml
name: plan
on:
  workflow_call:
    inputs:
      force: { type: boolean, default: false }
    outputs:
      php_matrix:  { value: ${{ jobs.plan.outputs.php_matrix  }} }
      ext_matrix:  { value: ${{ jobs.plan.outputs.ext_matrix  }} }
      tool_matrix: { value: ${{ jobs.plan.outputs.tool_matrix }} }

jobs:
  plan:
    runs-on: ubuntu-latest
    outputs:
      php_matrix:  ${{ steps.plan.outputs.php_matrix  }}
      ext_matrix:  ${{ steps.plan.outputs.ext_matrix  }}
      tool_matrix: ${{ steps.plan.outputs.tool_matrix }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - name: Build planner
        run: go build -o /tmp/planner ./cmd/planner
      - id: plan
        env:
          GHCR_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          FORCE: ${{ inputs.force }}
        run: |
          /tmp/planner \
            --catalog ./catalog \
            --lockfile ./bundles.lock \
            --registry ghcr.io/${{ github.repository_owner }} \
            --output-matrix /tmp/matrix \
            ${FORCE:+--force}
          echo "php_matrix=$(cat /tmp/matrix/php.json)"   >> "$GITHUB_OUTPUT"
          echo "ext_matrix=$(cat /tmp/matrix/ext.json)"   >> "$GITHUB_OUTPUT"
          echo "tool_matrix=$(cat /tmp/matrix/tool.json)" >> "$GITHUB_OUTPUT"
```

### 12.2 `build-extension.yml` (reusable)

```yaml
name: build-extension
on:
  workflow_call:
    inputs:
      extension: { required: true, type: string }  # "redis"
      version:   { required: true, type: string }  # "6.2.0"
      php_abi:   { required: true, type: string }  # "8.5-nts"
      os:        { required: true, type: string }  # "linux" | "macos" | "windows"
      arch:      { required: true, type: string }  # "x86_64" | "aarch64"
      digest:    { required: true, type: string }  # expected content digest

jobs:
  build:
    runs-on: >-
      ${{ fromJSON('{
        "linux-x86_64":"ubuntu-24.04",
        "linux-aarch64":"ubuntu-24.04-arm",
        "macos-arm64":"macos-14",
        "macos-x86_64":"macos-14-intel",
        "windows-x86_64":"windows-2022"
      }')[format('{0}-{1}', inputs.os, inputs.arch)] }}
    permissions:
      packages: write
      id-token: write       # for cosign keyless signing
    steps:
      - uses: actions/checkout@v4

      - name: Fetch prerequisite PHP core bundle from GHCR
        run: ./builders/common/fetch-core.sh "${{ inputs.php_abi }}" "${{ inputs.os }}" "${{ inputs.arch }}"

      - name: Build extension
        run: ./builders/${{ inputs.os }}/build-ext.sh
        env:
          EXT_NAME:    ${{ inputs.extension }}
          EXT_VERSION: ${{ inputs.version }}
          PHP_ABI:     ${{ inputs.php_abi }}

      - name: Package bundle (tar + zstd)
        run: ./builders/common/pack-bundle.sh /tmp/out /tmp/bundle.tar.zst

      - name: Verify digest matches plan
        run: |
          actual=$(sha256sum /tmp/bundle.tar.zst | awk '{print $1}')
          expected="${{ inputs.digest }}"
          [ "$actual" = "${expected#sha256:}" ] || {
            echo "digest mismatch: actual=$actual expected=$expected"
            exit 1
          }

      - name: Push to GHCR
        env:
          GHCR_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          echo "$GHCR_TOKEN" | oras login ghcr.io -u ${{ github.actor }} --password-stdin
          oras push \
            ghcr.io/${{ github.repository_owner }}/php-ext-${{ inputs.extension }}:${{ inputs.version }}-${{ inputs.php_abi }}-${{ inputs.os }}-${{ inputs.arch }} \
            --artifact-type application/vnd.buildrush.php-ext.v1 \
            /tmp/bundle.tar.zst:application/vnd.oci.image.layer.v1.tar+zstd \
            ./meta.json:application/vnd.buildrush.php-ext.meta.v1+json

      - uses: sigstore/cosign-installer@v3
      - name: Sign bundle
        run: |
          cosign sign --yes \
            ghcr.io/${{ github.repository_owner }}/php-ext-${{ inputs.extension }}@${{ inputs.digest }}

      - name: Smoke test
        run: ./test/smoke/run.sh "${{ inputs.extension }}" "${{ inputs.digest }}"
```

### 12.3 `on-push.yml` — the common-case dispatcher

```yaml
name: on-push
on:
  push:
    branches: [main]
    paths: ['catalog/**', 'builders/**']
  pull_request:
    paths: ['catalog/**', 'builders/**']

jobs:
  plan:
    uses: ./.github/workflows/plan.yml

  build-php:
    needs: plan
    if: ${{ needs.plan.outputs.php_matrix != '{"include":[]}' }}
    strategy:
      fail-fast: false
      max-parallel: 20
      matrix: ${{ fromJSON(needs.plan.outputs.php_matrix) }}
    uses: ./.github/workflows/build-php-core.yml
    with:
      version: ${{ matrix.version }}
      os:      ${{ matrix.os }}
      arch:    ${{ matrix.arch }}
      ts:      ${{ matrix.ts }}
      digest:  ${{ matrix.digest }}

  build-ext:
    needs: [plan, build-php]
    if: ${{ needs.plan.outputs.ext_matrix != '{"include":[]}' }}
    strategy:
      fail-fast: false
      max-parallel: 30
      matrix: ${{ fromJSON(needs.plan.outputs.ext_matrix) }}
    uses: ./.github/workflows/build-extension.yml
    with:
      extension: ${{ matrix.extension }}
      version:   ${{ matrix.version }}
      php_abi:   ${{ matrix.php_abi }}
      os:        ${{ matrix.os }}
      arch:      ${{ matrix.arch }}
      digest:    ${{ matrix.digest }}

  build-tools:
    needs: plan
    if: ${{ needs.plan.outputs.tool_matrix != '{"include":[]}' }}
    strategy:
      fail-fast: false
      max-parallel: 10
      matrix: ${{ fromJSON(needs.plan.outputs.tool_matrix) }}
    uses: ./.github/workflows/build-tool.yml
    with:
      tool:    ${{ matrix.tool }}
      version: ${{ matrix.version }}
      digest:  ${{ matrix.digest }}

  update-lock:
    needs: [build-php, build-ext, build-tools]
    if: ${{ github.event_name == 'push' && !cancelled() && !failure() }}
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/update-lockfile.sh
      - uses: peter-evans/create-pull-request@v7
        with:
          commit-message: "chore: update bundles.lock"
          title: "chore: update bundles.lock"
          branch: bot/lockfile-update
          body: |
            Automated lockfile update from build pipeline run ${{ github.run_id }}.
```

### 12.4 `nightly.yml` — scheduled reconciliation

```yaml
name: nightly
on:
  schedule:
    - cron: "0 3 * * *"
  workflow_dispatch:

jobs:
  reconcile:
    uses: ./.github/workflows/on-push.yml
    with:
      force: false
```

### 12.5 `watch-php-releases.yml`

```yaml
name: watch-php-releases
on:
  schedule:
    - cron: "0 * * * *"
  workflow_dispatch:

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Poll php.net
        id: poll
        run: |
          curl -sSf https://www.php.net/releases/active > /tmp/active.json
          if ! cmp -s /tmp/active.json ./.state/php-versions.json; then
            cp /tmp/active.json ./.state/php-versions.json
            echo "changed=true" >> "$GITHUB_OUTPUT"
          fi
      - name: Commit state
        if: steps.poll.outputs.changed == 'true'
        run: |
          git config user.name  "phpup-bot"
          git config user.email "bot@buildrush.dev"
          git add .state/php-versions.json
          git commit -m "chore: observed new PHP release"
          git push
      - name: Dispatch core rebuild
        if: steps.poll.outputs.changed == 'true'
        uses: peter-evans/repository-dispatch@v3
        with:
          event-type: php-release-detected
```

---

## 13. Matrix size and storage footprint

### 13.1 PHP cores

- 13 PHP versions × 4 Linux/macOS OS-arch combos × 1 TS flavor = 52
- 13 PHP versions × 2 Windows arch combos × 2 TS flavors = 52
- **Total: ~100 core bundles.** Rebuilt on new PHP releases (~monthly) or runner image bumps (~quarterly).

### 13.2 Extensions — tier 1 (top 50)

- 50 extensions × 8 recent PHP ABIs × 5 OS/arch combos = ~2,000 bundles
- Some `exclude` entries trim this to ~1,600 in practice.
- Most extension versions are stable; rebuilt only on version bumps.

### 13.3 Extensions — tier 2 (opportunistic long-tail)

- ~150 additional extensions, built opportunistically as demand signals surface (see § 15).
- Not all ABIs for every tier-2 extension — only those actually requested.
- Expected steady-state: ~3,000 tier-2 bundles at maturity.

### 13.4 Tools

- ~20 tools × 3 versions each × 1 target = ~60 bundles. Most are phars (arch-independent).

### 13.5 Total storage

- ~2–3 GB on GHCR at steady state.
- GitHub's public-package storage is free; overflow to Cloudflare R2 (existing sponsor) if needed.

### 13.6 Build wall-time budgets

- **Full cold rebuild** (after bootstrap or major change): 6–10 hours with `max-parallel: 30`.
- **Typical PR-triggered incremental**: 5–30 minutes depending on scope.
- **Nightly reconciliation** (no-op case): under 1 minute.

---

## 14. Bootstrap and ongoing operation

### 14.1 Cold start

The project begins with no lockfile, no bundles, and no consumers. A one-time `bootstrap.yml` workflow is manually dispatched:

1. Runs the full matrix with `force: true`.
2. Commits the initial `bundles.lock`.
3. Tags the action as `v0.1.0-alpha`.
4. Publishes initial release notes.

After this, the normal pipeline takes over and the bootstrap workflow is retained only for catastrophic recovery.

### 14.2 Steady-state operation

- **Nightly**: usually zero work; occasional refresh of unpinned versions.
- **Monthly** (new PHP patch release): ~300 bundle rebuilds, ~2 hours.
- **Per PR**: 0–50 bundles, 5–15 minutes.
- **Quarterly** (GHCR GC): cleanup of untagged/unreferenced bundles.

### 14.3 Monitoring

- **GitHub Actions run history** is the primary dashboard.
- **A status page** published via GitHub Pages from a small periodic workflow that queries GHCR manifest availability and computes SLO numbers (P50/P95 install time, tier-1 hit rate, last successful nightly).
- **Alerting** via issues automatically opened by workflows on repeated failures. No external services required.

---

## 15. Extension long-tail strategy

Not every possible (PHP, extension, version) tuple can be precomputed. The strategy is a four-tier cascade:

- **Tier 1 — Prebuilt bundle** (target: 95% of runs). Top ~50 extensions, pinned versions, all supported ABIs. Instant install.
- **Tier 2 — Prebuilt layer, composed locally.** Less-popular extensions built as individual artifacts; the action composes them into the core at runtime. Still no compile on the consumer runner.
- **Tier 3 — PECL fallback.** For obscure or brand-new versions not yet in the matrix, the action falls back to `pecl install` using dev headers shipped in the core bundle. Emits a warning with a link suggesting the extension be added to the catalog.
- **Tier 4 — Git source build.** For extensions hosted only on git, explicit opt-in via `extensions: foo@git+https://github.com/...`.

### 15.1 Auto-promotion loop

When the action hits a tier-3 fallback, it emits an optional, rate-limited, structured event via `repository_dispatch` to `buildrush/setup-php`:

```json
{
  "event_type": "tier3-observation",
  "client_payload": {
    "extension": "foo",
    "version": "1.2.3",
    "php": "8.5",
    "os": "linux",
    "arch": "x86_64"
  }
}
```

Users can opt out with `telemetry: false` in the action inputs.

A weekly aggregator workflow collects these events. When any (extension, version) pair crosses a threshold (e.g. 50 unique workflow runs across the ecosystem), a bot opens a PR adding a minimal catalog entry. Once merged, future runs hit tier 1 for that extension.

**The long tail promotes itself based on real demand.**

---

## 16. Versioning and reproducibility

### 16.1 Three tiers of action references

- **`buildrush/setup-php@v1`** — rolling tag, always points at the latest stable. Most users.
- **`buildrush/setup-php@v1.4.2`** — pinned semver. Recommended for reproducibility-sensitive and security-sensitive users; pairs well with Dependabot.
- **`buildrush/setup-php@main`** — bleeding edge, not recommended outside this repo's own CI.

### 16.2 The action–lockfile binding

Each action release tag is atomically bound to a specific `bundles.lock`. Specifically:

- Releasing `v1.4.2` means: compile the Go binary with the current `bundles.lock` embedded, build per-OS binaries, attach them to the GitHub Release, tag the commit.
- A bundle-only change (e.g. nightly promotes `latest-6.x` redis from `6.3.0` to `6.3.1`) still produces a new patch version, because the action's effective behavior changed.

### 16.3 Reproducibility guarantee

> Users who pin `buildrush/setup-php@v1.4.2` get byte-identical CI runs a year later, as long as GHCR still serves the referenced digests.

This is a guarantee `shivammathur/setup-php` cannot make today, because its upstream packages (`ppa:ondrej/php`, Homebrew taps, PECL) can and do change.

### 16.4 GHCR retention policy

A quarterly `gc-bundles.yml` workflow deletes:

- Untagged digests older than 30 days (failed or superseded builds).
- Bundles referenced nowhere in `bundles.lock` AND not pulled in the last 180 days.

It **never** deletes signed, lockfile-referenced bundles — those are load-bearing for the reproducibility guarantee.

---

## 17. Performance targets

| Scenario                                                 | `shivammathur/setup-php` today | `buildrush/setup-php` target |
| -------------------------------------------------------- | ------------------------------ | ---------------------------- |
| PHP 8.5 + composer, warm cache hit on `ubuntu-latest`    | 15–25 s                        | **1–2 s**                    |
| PHP 8.5 + 5 common extensions + composer + phpunit, cold | 30–60 s                        | **5–10 s**                   |
| PHP 8.3 on `ubuntu-24.04` (the current slow path)        | 2–3 min                        | **5–10 s**                   |
| Exotic extension (intl with ICU 77)                      | 60–120 s                       | **~5 s** (prebuilt bundle)   |
| Unknown PECL extension (tier 3 fallback)                 | 60–120 s                       | 60–120 s first run, **~5 s** subsequent |
| Windows PHP + PECL extension                             | 30–45 s                        | **5–10 s**                   |

Cold-path ceiling is bounded by bundle download bandwidth, which on GitHub-hosted runners to GHCR is ~200 MB/s. A 30 MB full bundle set downloads in well under a second; the rest is extraction and symlinking.

---

## 18. Migration path

### 18.1 Phase 0 — Parity alpha

- Build the runtime binary, the pipeline workflows, and the top-20 extensions × top-3 PHP versions × all OS/arch combos.
- Dogfood on a handful of volunteer OSS projects under `buildrush/setup-php@v0.x`.
- Goal: validate the approach with real workflows.

### 18.2 Phase 1 — Public beta

- Publish `buildrush/setup-php@v0`.
- Input compatibility: every documented input of `shivammathur/setup-php@v2` works identically.
- Provide `buildrush/migrate` CLI that rewrites `.github/workflows/*.yml` references in place and flags any unsupported inputs.
- Goal: any drop-in migration works with zero behavior changes.

### 18.3 Phase 2 — Full matrix

- All four tiers operational.
- Every extension and tool in the current shivammathur/setup-php catalog covered.
- Tag `v1.0.0`.

### 18.4 Phase 3 — Long-tail self-promotion

- Tier-3 observation and auto-promotion loop running.
- Target tier-1 hit rate >99%.

### 18.5 Phase 4 — Upstream collaboration

- Approach Shivam Mathur about folding the implementation into `shivammathur/setup-php@v3` (preserving the brand and accumulated trust) or co-maintaining.
- The goal is a coordinated transition, not a fork war.

---

## 19. Tradeoffs and risks

### 19.1 Accepted tradeoffs

- **Bundle matrix is real storage.** ~2–3 GB on GHCR. Free for public repos; Cloudflare R2 overflow if needed.
- **Bus factor shifts from runtime code to pipeline.** If the build pipeline stops running, bundles go stale. Mitigation: the pipeline is infrastructure-as-code in the public repo; anyone can fork and run it independently.
- **Initial matrix build is weeks of release engineering.** This is front-loaded work that is then amortized across every consumer run forever.
- **Tier-3 fallback must exist forever.** The long tail is real; pretending otherwise is how you ship an incomplete product.

### 19.2 Acknowledged risks

- **macOS and Windows are harder than Linux.** macOS arm64 requires codesigning dylibs; Windows requires VC++ runtime coordination. The existing `mlocati/powershell-phpmanager` work is genuinely good — best path is probably to reuse its logic inside the build pipeline rather than reinvent it.
- **Nightly PHP builds are inherently fragile.** They remain tier 3 indefinitely.
- **GHCR is a single point of trust.** Mitigated by the R2 mirror, by cosign signatures, and by content addressing (consumers can verify what they downloaded).
- **Binary ABI drift across runner image updates.** Mitigated by the `watch-runner-images.yml` workflow and by including the base runner image digest in the bundle digest computation — so a runner change forces new bundle digests.

### 19.3 Non-risks (deliberately)

- **Not losing compatibility.** The public API is preserved by construction; CI-level integration tests assert parity.
- **Not inventing a new registry.** GHCR is standard OCI; any OCI client can inspect and verify the artifacts.
- **Not depending on external build infrastructure.** Every workflow runs on standard GitHub-hosted runners.

---

## 20. Open questions

These are worth deciding before implementation starts:

1. **Go vs Rust for the runtime binary.** Leaning Go; invite dissent.
2. **Do we ship debug-symbol bundles separately, or always included?** Current action has a `debug` env flag; could be a separate bundle kind (`php-core-debug`) to keep the default small.
3. **How aggressive is the auto-promotion threshold?** 50 unique-workflow observations per week is a starting point; needs calibration.
4. **Do we publish the Go binary as a separate `buildrush/phpup` CLI** for local-dev use, or keep it embedded in the action only? Publishing it opens a second distribution surface but closes the local-parity gap more cleanly.
5. **Cosign key strategy.** Keyless OIDC is simple but ties trust to GitHub. Some users want traditional key-based signing for air-gapped mirrors.
6. **How to handle Windows ARM64** when GitHub adds arm64 Windows runners (expected 2026–2027).

---

## One-sentence summary

Keep `shivammathur/setup-php`'s public API verbatim; replace the guts with a Go binary that downloads content-addressed, signed OCI bundles from GHCR; build those bundles in the same repository via six trigger-specific workflows that funnel into three reusable per-kind build workflows; gate every publish on a content-digest check, a smoke test, and an auto-PR to a lockfile that the action binary embeds — so most pipeline triggers do zero work, incremental builds finish in minutes, user installs drop from 30+ seconds to 3–10 seconds, and every pinned action version remains reproducible for as long as the registry exists.
