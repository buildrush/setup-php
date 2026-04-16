# `buildrush/setup-php` — Phased Implementation Design

A specification for incrementally delivering the `buildrush/setup-php` GitHub Action described in `docs/product-vision.md`, starting with a Linux-only proof-of-concept and expanding through 7 phases to full cross-platform parity with `shivammathur/setup-php`.

---

## Table of Contents

1. [Context](#1-context)
2. [Phase Map](#2-phase-map)
3. [Phase 1: Linux PoC + Full Pipeline](#3-phase-1-linux-poc--full-pipeline)
4. [Phase 2: Linux Matrix Expansion](#4-phase-2-linux-matrix-expansion)
5. [Phase 3: Tools & Tiered Fallbacks](#5-phase-3-tools--tiered-fallbacks)
6. [Phase 4: Status Tracking & Developer Experience](#6-phase-4-status-tracking--developer-experience)
7. [Phase 5: macOS Support](#7-phase-5-macos-support)
8. [Phase 6: Windows Support](#8-phase-6-windows-support)
9. [Phase 7: Auto-promotion & Hardening](#9-phase-7-auto-promotion--hardening)
10. [Status Tracking Format](#10-status-tracking-format)
11. [Open Decisions per Phase](#11-open-decisions-per-phase)
12. [Verification Strategy](#12-verification-strategy)

---

## 1. Context

### Why this spec exists

`docs/product-vision.md` describes the full architecture of `buildrush/setup-php` — a ground-up reimplementation of `shivammathur/setup-php` that replaces runtime package-manager orchestration with content-addressed, prebuilt OCI bundles on GHCR, driven by a Go binary and a declarative catalog.

The vision is comprehensive (~950 lines) but monolithic. This spec decomposes it into 7 ordered implementation phases, each with a clear scope, deliverable, and exit criteria. The goal: each phase produces a shippable artifact, and no phase requires speculative work for a future phase.

### Guiding decisions

- **Linux first.** macOS and Windows are deferred to Phases 5 and 6. All Phase 1–4 work targets Linux x86_64 (expanding to aarch64 in Phase 2).
- **Full pipeline from day one.** All 6 trigger types (push, nightly, PHP release watch, runner image watch, manual dispatch, security advisory) are implemented in Phase 1, even though the matrix is tiny. The pipeline is the hardest infrastructure to get right; exercising it early with a small matrix catches design flaws cheaply.
- **Go from the start.** No shell prototype. The Go runtime binary (`phpup`) and the planner tool are built in Phase 1.
- **Status tracking is auto-generated.** A script reads `bundles.lock` and `catalog/` to produce per-platform status markdown files. No manual tracking.

### What already exists

The repository is greenfield — only `LICENSE` (MIT, Build_Rush copyright) and `docs/product-vision.md` exist. No source code, no configuration, no tests.

---

## 2. Phase Map

| Phase | Name | Scope | Depends On | Deliverable |
|-------|------|-------|------------|-------------|
| 1 | Linux PoC + Full Pipeline | Go runtime + planner, all 6 trigger workflows, PHP 8.4 NTS, 5 extensions, Linux x86_64, GHCR + cosign, lockfile | — | `buildrush/setup-php@v0.1.0-alpha` on `ubuntu-24.04` |
| 2 | Linux Matrix Expansion | PHP 8.1–8.5, top-50 extensions, aarch64, Ubuntu 22.04 + 24.04 | Phase 1 | Full Linux coverage for common workflows |
| 3 | Tools & Tiered Fallbacks | Tool bundles (composer, phpunit, phpstan, etc.), Tier 2/3/4 cascade, PECL fallback | Phase 2 | Complete Linux action with tools |
| 4 | Status Tracking & DX | Auto-generated status matrices, `phpup doctor`, `dry-run`, `mode` input, `cache-key` output | Phase 1 | Status pages + developer UX features |
| 5 | macOS | arm64 + x86_64, macOS 14/15/26 runners, dylib codesigning | Phase 3 | macOS support |
| 6 | Windows | x86_64, Windows Server 2022/2025, VC++ runtime, DLL extensions | Phase 3 | Windows support |
| 7 | Auto-promotion & Hardening | Tier-3 observation loop, auto-promotion PRs, R2 fallback mirror, GC workflow, security-rebuild | Phase 3 | Production-hardened system |

Phases 4–7 can run in parallel once Phase 3 is complete. Phase 4 only truly depends on Phase 1 (needs lockfile and catalog to exist).

### Version milestones

| Phase Complete | Action Version |
|----------------|---------------|
| 1 | `v0.1.0-alpha` |
| 2 | `v0.2.0-alpha` |
| 3 | `v0.3.0-beta` |
| 4 | `v0.4.0-beta` |
| 5 | `v0.5.0-beta` |
| 6 | `v0.6.0-beta` |
| 7 | `v1.0.0` |

---

## 3. Phase 1: Linux PoC + Full Pipeline

### 3.1 Goal

Prove the entire architecture end-to-end with the smallest possible matrix: a user can write `uses: buildrush/setup-php@v0.1.0-alpha` with `php-version: '8.4'` and `extensions: mbstring, intl, curl, zip, redis` on an `ubuntu-24.04` runner and get a working PHP environment in under 10 seconds.

### 3.2 Target matrix

| Dimension | Values |
|-----------|--------|
| PHP versions | 8.4 (latest patch) |
| Thread safety | NTS |
| OS | Linux |
| Arch | x86_64 |
| Runner | `ubuntu-24.04` |
| Extensions | mbstring (bundled), intl (ICU), curl (libcurl), zip (libzip), redis (PECL) |
| Tools | None |

Total bundles: **1 PHP core + 1 extension bundle** (mbstring, intl, curl, zip are compiled into core via configure flags; only redis requires a separate PECL-built bundle) = **2 bundles**.

### 3.3 Components

#### A. Go Runtime Binary — `cmd/phpup/`

The action's core binary. Cross-compiled for Linux x86_64 in Phase 1 (macOS/Windows targets added in Phases 5/6).

**Packages:**

| Package | Responsibility |
|---------|---------------|
| `cmd/phpup/main.go` | Entry point, parse CLI flags and GitHub Actions env vars |
| `internal/plan/` | Parse `INPUT_*` env vars into a canonical `Plan` struct |
| `internal/resolve/` | Look up plan against embedded `bundles.lock`, produce list of OCI digests |
| `internal/oci/` | Parallel HTTP/2 fetch from GHCR, digest verification, cosign signature verification |
| `internal/extract/` | Zstd decompression, tar extraction to target directories |
| `internal/compose/` | Symlink extension `.so` into core's `extension_dir`, write `conf.d/*.ini` fragments |
| `internal/env/` | Write `GITHUB_ENV`, `GITHUB_OUTPUT`, `GITHUB_PATH` files |
| `internal/cache/` | Check/seed `$RUNNER_TOOL_CACHE/buildrush/<plan-hash>` |

**Key dependencies:**
- `github.com/google/go-containerregistry` — OCI registry client
- `github.com/klauspost/compress/zstd` — zstd decompression
- `github.com/sigstore/cosign/v2` — signature verification
- `gopkg.in/yaml.v3` — lockfile/catalog parsing

**Runtime flow (happy path):**

```
Parse inputs → Resolve against lockfile → Check tool cache
  ├─ HIT  → Symlink cached tree → Export env → Exit (1-2s)
  └─ MISS → Parallel OCI fetch → Verify digests → Verify cosign
            → Parallel extract → Compose → Export env → Seed cache → Exit (5-10s)
```

**Input compatibility:** The binary accepts the same inputs as `shivammathur/setup-php@v2`:
- `php-version` — resolved to exact patch version
- `extensions` — comma-separated list
- `ini-values` — comma-separated key=value pairs
- `coverage` — `xdebug`, `pcov`, or `none`
- `tools` — ignored in Phase 1 (warning emitted)
- `php-version-file` — `.php-version` file support
- Environment flags: `fail-fast`, `phpts`, `update`, `debug`, `verbose`

Inputs not yet supported emit a clear warning message, not a failure (unless `fail-fast` is set).

#### B. Planner Tool — `cmd/planner/`

Runs in CI to produce build matrices. Not shipped to users.

**Responsibilities:**
1. Parse all YAML files in `catalog/`
2. Expand `abi_matrix` into concrete cells, apply `exclude` rules
3. Compute spec hash per cell: `sha256(canonical_yaml || builder_script_hash || runner_image_digest || tool_versions)`
4. Diff against `bundles.lock` — cells whose spec hash matches a lockfile entry AND whose digest exists on GHCR are skipped
5. Output three matrix JSON files (`php.json`, `ext.json`, `tool.json`) for consumption by GitHub Actions `fromJSON()`
6. Support `--force` flag to bypass skip-if-present logic

**CLI interface:**
```
planner \
  --catalog ./catalog \
  --lockfile ./bundles.lock \
  --registry ghcr.io/buildrush \
  --output-matrix /tmp/matrix \
  [--force]
```

#### C. Catalog Files

**`catalog/php.yaml`** — PHP 8.4 core build recipe:
```yaml
name: php
versions:
  - "8.4"                           # resolved to latest patch by planner
source:
  url: "https://www.php.net/distributions/php-{version}.tar.xz"
  sig: "https://www.php.net/distributions/php-{version}.tar.xz.asc"
abi_matrix:
  os:   ["linux"]
  arch: ["x86_64"]
  ts:   ["nts"]
configure_flags:
  common: >-
    --enable-mbstring
    --with-curl
    --with-zlib
    --with-openssl
    --enable-bcmath
    --enable-calendar
    --enable-exif
    --enable-ftp
    --enable-intl
    --with-zip
    --enable-soap
    --enable-sockets
    --with-pdo-mysql
    --with-pdo-sqlite
    --with-sqlite3
    --with-readline
    --with-sodium
    --enable-gd
    --with-freetype
    --with-jpeg
    --with-webp
  linux: >-
    --with-pdo-pgsql
    --with-pgsql
bundled_extensions:
  - mbstring
  - curl
  - intl
  - zip
  - json
  - pdo
  - pdo_mysql
  - pdo_sqlite
  - sqlite3
  - tokenizer
  - xml
  - dom
  - simplexml
  - xmlreader
  - xmlwriter
  - ctype
  - filter
  - hash
  - iconv
  - session
  - bcmath
  - calendar
  - exif
  - ftp
  - soap
  - sockets
  - sodium
  - gd
  - readline
  - openssl
  - zlib
  - opcache
  - pgsql
  - pdo_pgsql
smoke:
  - 'php -v'
  - 'php -m'
  - 'php -r "echo PHP_VERSION;"'
```

**`catalog/extensions/redis.yaml`** — PECL extension example:
```yaml
name: redis
kind: pecl
source:
  pecl_package: redis
versions:
  - "6.2.0"
abi_matrix:
  php:  ["8.4"]
  os:   ["linux"]
  arch: ["x86_64"]
  ts:   ["nts"]
runtime_deps:
  linux: []
ini:
  - "extension=redis"
smoke:
  - 'php -r "assert(extension_loaded(\"redis\"));"'
```

**`catalog/extensions/intl.yaml`** — bundled with core in Phase 1 (built via configure flags, not a separate bundle). Catalog entry exists for documentation and future tier-2 use:
```yaml
name: intl
kind: bundled
note: "Built into PHP core via --enable-intl. Separate bundle not needed unless custom ICU version requested."
```

Similarly for `mbstring.yaml`, `curl.yaml`, `zip.yaml` — all bundled with core.

**Net effect for Phase 1:** Only **redis** produces a separate extension bundle. The other 4 extensions are compiled into the PHP core bundle via configure flags. Total bundles: **1 core + 1 extension = 2 bundles**.

#### D. Builder Scripts

**`builders/common/fetch-core.sh`**
- Downloads the PHP core bundle from GHCR using `oras pull`
- Extracts to a known path (`/opt/buildrush/core/`)
- Makes phpize, php-config available for extension builds

**`builders/common/pack-bundle.sh`**
- Takes a directory, creates `tar --zstd` archive
- Computes SHA-256 digest
- Writes metadata sidecar JSON (name, version, abi, build timestamp, builder versions)

**`builders/linux/build-php.sh`**
- Installs build dependencies via `apt-get` (autoconf, bison, re2c, libicu-dev, libcurl4-openssl-dev, libzip-dev, etc.)
- Downloads PHP source tarball, verifies GPG signature
- Runs `./configure` with flags from catalog
- `make -j$(nproc)` and `make install INSTALL_ROOT=/tmp/out`
- Strips binaries
- Runs `pack-bundle.sh`

**`builders/linux/build-ext.sh`**
- Fetches prerequisite PHP core from GHCR
- If `kind: pecl`: `pecl download $EXT_NAME-$EXT_VERSION`, extract, phpize, configure, make, make install
- Collects the `.so` file and any runtime deps into output directory
- Runs `pack-bundle.sh`

#### E. GitHub Actions Workflows

All 6 trigger types plus supporting workflows:

| Workflow | Type | Purpose |
|----------|------|---------|
| `plan.yml` | Reusable | Diff catalog vs lockfile, emit build matrices |
| `build-php-core.yml` | Reusable | Build one PHP core bundle |
| `build-extension.yml` | Reusable | Build one extension bundle |
| `on-push.yml` | Trigger | Catalog/builder changes on push or PR |
| `nightly.yml` | Trigger | Scheduled daily reconciliation (`0 3 * * *`) |
| `watch-php-releases.yml` | Trigger | Hourly poll of `php.net/releases/active` |
| `watch-runner-images.yml` | Trigger | Daily poll of `actions/runner-images` changelog |
| `manual.yml` | Trigger | `workflow_dispatch` with extension/version/force inputs |
| `security-rebuild.yml` | Trigger | `repository_dispatch` for CVE-driven rebuilds |
| `release-action.yml` | Supporting | Cross-compile Go binary, create GitHub Release, tag |
| `gc-bundles.yml` | Supporting | Stub in Phase 1; retention cleanup logic added in Phase 7 |
| `bootstrap.yml` | Supporting | One-time cold start: force-build entire matrix |

**Workflow architecture:**
```
Triggers (on-push, nightly, watch-*, manual, security-rebuild)
    │
    ▼
plan.yml (reusable) → matrix JSONs
    │
    ├─► build-php-core.yml (reusable, per matrix cell)
    │       ├─ Build → Pack → Verify digest → Push GHCR → Cosign sign → Smoke test
    │
    └─► build-extension.yml (reusable, per matrix cell)
            ├─ Fetch core → Build ext → Pack → Verify digest → Push GHCR → Cosign sign → Smoke test
    │
    ▼
update-lockfile (auto-PR to main)
```

#### F. Action Entry Point

**`action.yml`:**
```yaml
name: 'Setup PHP'
description: 'Fast, reproducible PHP setup with prebuilt bundles'
inputs:
  php-version:
    description: 'PHP version to set up'
    required: false
    default: '8.4'
  extensions:
    description: 'Comma-separated list of PHP extensions'
    required: false
  ini-values:
    description: 'Comma-separated list of php.ini values'
    required: false
  coverage:
    description: 'Coverage driver (xdebug, pcov, none)'
    required: false
    default: 'none'
  tools:
    description: 'Comma-separated list of tools'
    required: false
  php-version-file:
    description: 'File containing PHP version'
    required: false
  github-token:
    description: 'GitHub token for authenticated registry access'
    required: false
    default: ${{ github.token }}
outputs:
  php-version:
    description: 'Resolved PHP version'
runs:
  using: 'node20'
  main: 'dist/index.js'
```

**`src/index.js`** — thin Node.js bootstrap (~30 lines):
- Determines runner OS and arch
- Downloads the correct Go binary from the action's GitHub Release (cached in `$RUNNER_TOOL_CACHE`)
- Execs the Go binary with the action inputs forwarded as env vars

#### G. Lockfile

**`bundles.lock`** — generated by bootstrap, updated by pipeline:
```json
{
  "schema_version": 1,
  "generated_at": "2026-04-16T03:00:00Z",
  "bundles": {
    "php:8.4.x:linux:x86_64:nts": "sha256:...",
    "ext:redis:6.2.0:8.4:linux:x86_64:nts": "sha256:..."
  }
}
```

Compiled into the Go binary at release time via `go:embed`.

#### H. Tests

**`test/smoke/run.sh`** — given a bundle name and digest, pulls from GHCR, extracts, runs the `smoke:` commands from the catalog entry.

**`test/integration/`** — a GitHub Actions workflow that:
1. Uses `buildrush/setup-php@<current-branch>` with `php-version: '8.4'` and `extensions: mbstring, intl, curl, zip, redis`
2. Asserts `php -v` outputs PHP 8.4.x
3. Asserts `php -m` lists all 5 extensions
4. Runs `php -r "echo json_encode(get_loaded_extensions());"` and validates
5. Runs a basic Composer install (if tools are available) or a phpinfo script

**Go unit tests:**
- `internal/plan/` — input parsing edge cases
- `internal/resolve/` — lockfile lookup, missing entries
- `internal/oci/` — digest verification (using test fixtures)
- `internal/compose/` — symlink logic, ini fragment generation

#### I. Developer Experience & QA Tooling

All tooling is set up in Phase 1 and enforced in CI from day one. Contributors must be able to run the full check suite locally before pushing.

**Go tooling (strict config, CI-enforced):**
- `golangci-lint` with a strict `.golangci.yml` config enabling: `govet`, `gofmt`, `goimports`, `errcheck`, `staticcheck`, `gosec`, `ineffassign`, `typecheck`, `unused`, `misspell`, `revive`, `gocritic`, `exhaustive`, `nilerr`, `prealloc`
- `go vet ./...` — built-in static analysis
- `gofmt -s` — enforced formatting, zero tolerance for drift
- `go test -race -cover ./...` — race detector on all tests
- `go mod tidy` — verified clean in CI (diff check on `go.sum`)

**Node.js tooling (for thin bootstrap + any scripting):**
- `eslint` with strict TypeScript config
- `prettier` for formatting
- Surface is deliberately minimal (the `src/index.js` bootstrap is ~30 lines)

**CI enforcement — `ci-lint.yml` workflow:**
- Runs on every PR targeting `main`
- Checks: `golangci-lint run`, `go vet`, `gofmt` diff check, `go test -race`, `go mod tidy` diff check, `eslint`, `prettier --check`
- Merge is blocked if any check fails (branch protection rule on `main`)
- Separate from the build pipeline — fast feedback loop (~30 seconds)

**Makefile for local convenience:**

```makefile
.PHONY: check fmt vet lint tidy test build clean

# Run all checks locally (mirrors CI exactly)
check: fmt-check vet lint tidy-check test build

# Format all code
fmt:
	gofmt -s -w .
	npx prettier --write src/

# Check formatting without modifying (CI mode)
fmt-check:
	@test -z "$$(gofmt -s -l .)" || (gofmt -s -l . && echo "gofmt: files need formatting" && exit 1)
	npx prettier --check src/

# Go vet
vet:
	go vet ./...

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...
	npx eslint src/

# Verify go.sum is clean
tidy:
	go mod tidy

tidy-check:
	go mod tidy
	@git diff --exit-code go.sum go.mod || (echo "go.sum/go.mod is dirty after tidy" && exit 1)

# Test with race detector
test:
	go test -race -cover ./...

# Build all binaries (native platform)
build:
	go build -o bin/phpup ./cmd/phpup
	go build -o bin/planner ./cmd/planner

# Cross-compile for Linux (for local development on macOS/Windows)
build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/phpup-linux-amd64 ./cmd/phpup

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/phpup-linux-arm64 ./cmd/phpup

# Build a PHP core bundle locally via Docker
bundle-php:
	docker run --rm \
		-v $$(pwd):/workspace -w /workspace \
		-e PHP_VERSION=$(PHP_VERSION) \
		-e ARCH=$(ARCH) \
		ubuntu:24.04 \
		./builders/linux/build-php.sh

# Build an extension bundle locally via Docker
bundle-ext:
	docker run --rm \
		-v $$(pwd):/workspace -w /workspace \
		-e EXT_NAME=$(EXT_NAME) \
		-e EXT_VERSION=$(EXT_VERSION) \
		-e PHP_ABI=$(PHP_ABI) \
		ubuntu:24.04 \
		./builders/linux/build-ext.sh

# Clean build artifacts
clean:
	rm -rf bin/ /tmp/out /tmp/bundle.tar.zst
```

**Local cross-platform bundle building via Docker:**

Contributors build and test Linux bundles locally on any platform (macOS, Linux, Windows with Docker Desktop):

```bash
# Build PHP 8.4 core for Linux x86_64
make bundle-php PHP_VERSION=8.4 ARCH=x86_64

# Build redis extension for PHP 8.4
make bundle-ext EXT_NAME=redis EXT_VERSION=6.2.0 PHP_ABI=8.4-nts

# Run smoke tests on the local bundle
docker run --rm -v $(pwd)/out:/bundles ubuntu:24.04 \
  ./test/smoke/run.sh redis
```

Builder scripts are written to work identically inside Docker containers and on GitHub Actions runners. The Docker images match the CI runner images (ubuntu:22.04, ubuntu:24.04). macOS and Windows bundles cannot be built locally via Docker — those are CI-only (Phases 5/6).

**Release automation via plain workflow:**

The `release-action.yml` workflow handles the full release process without external tooling:

- Cross-compile the Go binary for target platforms (`linux/amd64` in Phase 1; more targets added in later phases)
- Embed `bundles.lock` into the binary via `go:embed`
- Stamp version and commit hash via `-ldflags`
- Generate `sha256sums.txt` checksum file
- Create GitHub Release via `gh release create` with auto-generated release notes
- Attach binary artifacts to the release
- Tag the action version (`v0.1.0-alpha`, etc.)

No external release tooling (e.g. GoReleaser) — a plain workflow with `go build` + `gh release create` keeps dependencies minimal and gives full control. Can migrate to GoReleaser later if the release matrix grows complex enough to justify it (e.g. Homebrew tap publishing for standalone CLI distribution).

**Documentation shipped in Phase 1:**

**`README.md`** — serves two audiences:
1. **Action users**: how to use `buildrush/setup-php` in GitHub Actions workflows, input reference, examples, performance comparison with `shivammathur/setup-php`, migration guide
2. **Contributors**: how to set up the development environment, run checks, build bundles locally, submit PRs

Structured as:
- Quick start (3-line workflow example)
- Input reference table (all inputs with defaults and descriptions)
- Extension support (what's available, how to request new ones)
- Performance (benchmarks vs shivammathur)
- Architecture overview (how bundles work, one paragraph)
- Contributing section (or link to `CONTRIBUTING.md`)
  - Prerequisites (Go 1.22+, Docker, Node.js)
  - Setup (`make check` to verify environment)
  - Adding an extension (edit catalog YAML, run `make bundle-ext`)
  - Running tests (`make test`)
  - PR process (CI checks must pass, no force-push to main)

**`CONTRIBUTING.md`** — detailed contributor guide:
- Development environment setup
- Repository structure explanation
- How to add extensions, PHP versions, tools
- Code style enforcement (all automated, no debates)
- PR review process
- Release process

**`CLAUDE.md`** — AI-assisted development constraints:
- All code changes must pass `make check` before committing
- All implementations must comply with the specs in `docs/superpowers/specs/`
- No "Co-Authored-By" AI attribution or similar trailing text in commit messages or PR descriptions
- Follow existing code patterns and conventions
- Run smoke tests for any bundle-related changes
- Reference the product vision (`docs/product-vision.md`) and phase spec for architectural decisions

**Configuration files shipped in Phase 1:**
- `.golangci.yml` — strict linter config
- `.eslintrc.json` — strict ESLint config for Node.js
- `.prettierrc` — Prettier config
- `Makefile` — local convenience commands
- `.editorconfig` — consistent editor settings (indent, line endings, trailing whitespace)
- `.github/workflows/ci-lint.yml` — PR lint/test enforcement
- `README.md` — user and contributor documentation
- `CONTRIBUTING.md` — detailed contributor guide
- `CLAUDE.md` — AI-assisted development constraints

### 3.4 Exit Criteria

Phase 1 is complete when:

1. `bootstrap.yml` runs successfully and populates `bundles.lock` with 2 bundle entries (1 core, 1 redis)
2. All 6 trigger workflows are operational and tested
3. A user can write this workflow and it passes:
   ```yaml
   - uses: buildrush/setup-php@v0.1.0-alpha
     with:
       php-version: '8.4'
       extensions: mbstring, intl, curl, zip, redis
   - run: php -v && php -m
   - run: php -r "assert(extension_loaded('redis'));"
   ```
4. Cold install completes in under 10 seconds on `ubuntu-24.04`
5. Warm cache hit completes in under 2 seconds
6. All bundles are cosign-signed and signature-verified at runtime
7. Integration tests pass in CI
8. `v0.1.0-alpha` is tagged and released

### 3.5 Explicitly Not in Phase 1

- Tools (composer, phpunit, etc.)
- macOS, Windows
- aarch64 architecture
- PHP versions other than 8.4
- Tier 2/3/4 fallback logic
- Auto-promotion
- R2 fallback mirror
- `phpup doctor`, `dry-run`, `mode`, `cache-key`
- Status page generation
- Ubuntu 22.04 (only 24.04)

---

## 4. Phase 2: Linux Matrix Expansion

### 4.1 Goal

Expand from the Phase 1 proof-of-concept to cover the most common Linux CI configurations.

### 4.2 Scope

| Dimension | Phase 1 | Phase 2 |
|-----------|---------|---------|
| PHP versions | 8.4 | 8.1, 8.2, 8.3, 8.4, 8.5 |
| Extensions | 5 (4 bundled + redis) | Top 50 |
| Arch | x86_64 | x86_64 + aarch64 |
| Runner | ubuntu-24.04 | ubuntu-22.04 + ubuntu-24.04 |
| Thread safety | NTS | NTS (ZTS for select extensions) |

### 4.3 Key work items

- Add PHP 8.1–8.3, 8.5 entries to `catalog/php.yaml`
- Create catalog entries for top-50 extensions (using `shivammathur/setup-php`'s extension list as the baseline)
- Add aarch64 to `abi_matrix` in catalog entries
- Add `ubuntu-22.04` runner mapping to workflow matrices
- Extend builder scripts for aarch64 (cross-compilation or native ARM runners)
- Scale the build pipeline: higher `max-parallel`, test with larger matrices
- Handle per-PHP-version configure flag differences (PHP 8.1 vs 8.5 have different bundled extension sets)

### 4.4 Extension selection criteria (top 50)

Prioritized by download count on Packagist and frequency in `shivammathur/setup-php` issue reports:
- **Already bundled in core**: mbstring, intl, curl, zip, json, pdo, pdo_mysql, pdo_sqlite, sqlite3, xml, dom, simplexml, gd, opcache, sodium, bcmath, calendar, ctype, exif, filter, ftp, hash, iconv, session, soap, sockets, readline, openssl, zlib, tokenizer, xmlreader, xmlwriter, pgsql, pdo_pgsql
- **Separate PECL builds needed**: redis, xdebug, pcov, imagick, memcached, apcu, mongodb, igbinary, msgpack, amqp, swoole, grpc, protobuf, ssh2, yaml, uuid, event, rdkafka

### 4.5 Exit criteria

- `bundles.lock` contains entries for PHP 8.1–8.5 × x86_64 + aarch64 = 10 core bundles
- Top-50 extensions built and smoke-tested
- Integration tests pass on both `ubuntu-22.04` and `ubuntu-24.04`
- `v0.2.0-alpha` tagged

---

## 5. Phase 3: Tools & Tiered Fallbacks

### 5.1 Goal

Add developer tool support and the tiered fallback strategy so the action handles the full range of user requests gracefully.

### 5.2 Scope

**Tools:**
- `php-tool` bundle kind in catalog and builder pipeline
- Catalog entries for: composer (v2), phpunit, phpstan, psalm, phpcs, phpcbf, php-cs-fixer, pint, rector, infection, deployer, phpmd, parallel-lint, phive
- Tool bundles are mostly architecture-independent (PHAR files)
- Builder script: `builders/common/build-tool.sh` — download PHAR, verify GPG/SHA signature, package
- `build-tool.yml` reusable workflow

**Tiered fallbacks:**
- **Tier 1** — prebuilt bundle (already working from Phase 2)
- **Tier 2** — prebuilt extension layer, composed at runtime. For less-popular extensions that have catalog entries but aren't in the top-50. The action fetches the extension bundle separately and composes it into the core.
- **Tier 3** — PECL fallback. When no prebuilt bundle exists, the action falls back to `pecl install` using the dev headers shipped in the core bundle. Emits a warning with a link suggesting the extension be added to the catalog.
- **Tier 4** — Git source build. For `extensions: foo@git+https://...` syntax. Explicit opt-in only.

**Runtime changes to `cmd/phpup/`:**
- Tool resolution and installation logic
- Tier cascade: try tier 1 → 2 → 3 → 4, with configurable behavior via `fail-fast`
- Warning/info messages for tier 2+ fallbacks

### 5.3 Exit criteria

- All listed tools installable via `tools: composer, phpunit, phpstan`
- Tier 3 PECL fallback demonstrated for an extension not in the catalog
- `v0.3.0-beta` tagged

---

## 6. Phase 4: Status Tracking & Developer Experience

### 6.1 Goal

Auto-generate implementation status tracking from the source of truth (lockfile + catalog) and add developer-experience features.

### 6.2 Scope

**Status tracking:**
- Script (`cmd/status-gen/` or `scripts/generate-status.sh`) that reads `bundles.lock` and `catalog/` to produce status markdown
- Output: `status/linux-x86_64.md`, `status/linux-aarch64.md` (and later `status/macos-arm64.md`, etc.)
- Each file contains a matrix table: PHP versions (columns) × extensions (rows), with status indicators
- Workflow to regenerate on every lockfile update (part of the auto-PR pipeline)
- Published to GitHub Pages or checked into the repo

**DX features:**
- `phpup doctor` subcommand: prints resolved bundles, tool-cache state, disk usage, suggestions
- `dry-run: true` input: prints the resolved bundle plan without downloading
- `mode` input: `fast` (fail if no prebuilt), `hermetic` (ignore pre-installed PHP), `compat` (mirror shivammathur behavior)
- `cache-key` output: stable hash of the resolved plan, usable as cache key for downstream steps

### 6.3 Exit criteria

- Status markdown files auto-generated and accurate
- `phpup doctor` works locally and in CI
- `dry-run` mode outputs a human-readable plan
- `v0.4.0-beta` tagged

---

## 7. Phase 5: macOS Support

### 7.1 Goal

Extend the action to macOS runners (arm64 and x86_64).

### 7.2 Scope

- Cross-compile Go binary for `darwin/arm64` and `darwin/amd64`
- `builders/macos/build-php.sh` — compile PHP on macOS using Homebrew-installed build dependencies
- `builders/macos/build-ext.sh` — build extensions on macOS
- Handle dylib codesigning (`codesign --sign -`) for arm64 macOS
- Add macOS entries to `catalog/php.yaml` and extension catalog files
- Runner matrix: `macos-14` (arm64), `macos-15` (arm64), `macos-26` (arm64), `macos-14-intel` (x86_64)
- Handle macOS-specific configure flags and library paths (Homebrew prefix differences between Intel and ARM)
- Update status generation to include `status/macos-arm64.md` and `status/macos-x86_64.md`

### 7.3 Key challenges

- Homebrew library paths differ between Intel (`/usr/local`) and ARM (`/opt/homebrew`)
- Some extensions need macOS-specific patches (notably `imagick` with ImageMagick from Homebrew)
- Dylib codesigning is required for arm64 macOS — unsigned `.dylib` files won't load
- macOS runners are more expensive (per-minute cost)
- ICU library bundling on macOS requires careful handling

### 7.4 Exit criteria

- PHP 8.1–8.5 + top-50 extensions on macOS arm64 and x86_64
- Integration tests pass on `macos-14` and `macos-15`
- `v0.5.0-beta` tagged

---

## 8. Phase 6: Windows Support

### 8.1 Goal

Extend the action to Windows runners.

### 8.2 Scope

- Cross-compile Go binary for `windows/amd64`
- `builders/windows/build-php.ps1` — compile or download PHP for Windows
- `builders/windows/build-ext.ps1` — build/download DLL extensions
- VC++ runtime coordination (ensure correct Visual C++ redistributable is present)
- Handle NTS vs ZTS (Windows uses ZTS more frequently)
- Windows-specific extension DLLs from PECL or `windows.php.net`
- Runner matrix: `windows-2022`, `windows-2025`
- PowerShell-based composition logic in the Go binary (or cross-platform Go code with Windows path handling)
- Update status generation to include `status/windows-x86_64.md`

### 8.3 Key challenges

- PHP Windows builds use Visual Studio toolchain — cannot easily compile from source in CI
- Best approach: download official Windows PHP builds from `windows.php.net`, repackage as OCI bundles
- Extension DLLs must match the exact PHP build (VC version, thread safety, architecture)
- Reuse logic from `mlocati/powershell-phpmanager` where applicable (identified in product vision as a good approach)
- Windows path handling (backslashes, `php.ini` location, extension_dir format)

### 8.4 Exit criteria

- PHP 8.1–8.5 + top-50 extensions on Windows x86_64
- Both NTS and ZTS variants available
- Integration tests pass on `windows-2022` and `windows-2025`
- `v0.6.0-beta` tagged

---

## 9. Phase 7: Auto-promotion & Hardening

### 9.1 Goal

Complete the long-tail self-promotion loop and harden the system for production use at scale.

### 9.2 Scope

**Auto-promotion:**
- Tier-3 observation events: when the action falls back to PECL, emit a rate-limited `repository_dispatch` event with extension/version/PHP/OS/arch
- `telemetry: false` input to opt out
- Weekly aggregator workflow: collects tier-3 events, identifies (extension, version) pairs crossing the threshold (50 unique workflow runs)
- Bot opens a PR adding a minimal catalog entry for high-demand extensions
- Once merged, future runs hit tier 1

**Hardening:**
- R2 fallback mirror: configure Cloudflare R2 bucket as a secondary bundle source, selected at runtime when GHCR returns transient errors
- `gc-bundles.yml` operational: delete untagged digests >30 days, delete unreferenced bundles not pulled in 180 days, never delete lockfile-referenced bundles
- `security-rebuild.yml` fully operational: accept signed CVE payloads, force rebuild, gate promotion on maintainer review
- Cosign key strategy finalized: support both keyless OIDC and traditional key-based signing for air-gapped mirrors
- Rate limiting and retry logic hardened in the Go binary
- Error messages improved based on alpha/beta feedback

### 9.3 Exit criteria

- Auto-promotion loop demonstrated end-to-end (an extension promoted from tier 3 to tier 1 automatically)
- R2 fallback mirror operational and tested
- GC workflow running quarterly
- Security rebuild tested with a simulated CVE
- `v1.0.0` tagged

---

## 10. Status Tracking Format

Status is auto-generated from `bundles.lock` and `catalog/`. Each platform gets its own markdown file.

### 10.1 File structure

```
status/
├── linux-x86_64.md
├── linux-aarch64.md
├── macos-arm64.md        # Added in Phase 5
├── macos-x86_64.md       # Added in Phase 5
└── windows-x86_64.md     # Added in Phase 6
```

### 10.2 Format per file

```markdown
# Linux x86_64 — Bundle Status

> Auto-generated from `bundles.lock` and `catalog/` on 2026-04-16T03:00:00Z.
> Do not edit manually.

## PHP Core Bundles

| PHP Version | Thread Safety | Status | Digest |
|-------------|--------------|--------|--------|
| 8.4.x | NTS | :white_check_mark: Built | `sha256:3a1f...` |
| 8.3.x | NTS | :white_check_mark: Built | `sha256:b2c4...` |
| 8.2.x | NTS | :white_check_mark: Built | `sha256:e7a1...` |

## Extension Bundles

| Extension | 8.1 | 8.2 | 8.3 | 8.4 | 8.5 |
|-----------|-----|-----|-----|-----|-----|
| redis 6.2.0 | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| xdebug 3.4.0 | :x: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| imagick 3.7.0 | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :construction: |

## Tool Bundles

| Tool | Version | Status |
|------|---------|--------|
| composer | 2.7.7 | :white_check_mark: |
| phpstan | 1.11.0 | :white_check_mark: |

### Legend
- :white_check_mark: Built and smoke-tested
- :construction: In catalog, build pending
- :x: Not supported for this PHP version
- :grey_question: Not in catalog
```

### 10.3 Generation

A Go tool (`cmd/status-gen/`) or shell script reads:
1. `catalog/` — what should exist (expanded abi_matrix minus excludes)
2. `bundles.lock` — what does exist (digest entries)
3. Produces the diff: "in catalog but not in lockfile" = `:construction:`, "in lockfile" = `:white_check_mark:`, "excluded" = `:x:`

Integrated into the lockfile-update auto-PR workflow: every time `bundles.lock` is updated, the status files are regenerated and included in the PR.

---

## 11. Open Decisions per Phase

Decisions that must be resolved before or during each phase.

### Phase 1
- **Go module path**: `github.com/buildrush/setup-php` or a vanity import path?
- **Minimum Go version**: 1.22+ (for `go:embed` and recent standard library features)
- **OCI client library**: `go-containerregistry` vs `oras-go` — both are mature; `go-containerregistry` is more widely used in the GitHub ecosystem
- **Node.js bootstrap version**: `node20` (current GitHub Actions standard) or `node24` (what shivammathur uses)?
- **GHCR organization**: `ghcr.io/buildrush/` — does the `buildrush` org exist on GitHub?

### Phase 2
- **Which 50 extensions make the cut?** Use Packagist download stats + shivammathur issue frequency as the selection criteria.
- **ICU version strategy**: ship a single ICU version per PHP version, or allow ICU version selection (complicates the matrix)?
- **aarch64 build approach**: native ARM runners (`ubuntu-24.04-arm`) vs cross-compilation on x86_64?

### Phase 3
- **Tool version resolution**: tool versions are pinned in the catalog and resolved at build time (not runtime). The planner resolves `latest-1.x` style constraints during nightly reconciliation and produces a concrete version. The action binary only sees exact versions via the lockfile.
- **PECL fallback dev headers**: ship `phpize` and `php-config` in the core bundle (they're already there), but also ship the header files (`include/php/`)?
- **Tier-3 fallback needs build tools**: should the action install `build-essential` and `autoconf` on the runner, or fail with a message?

### Phase 5
- **macOS Homebrew dependency caching**: cache Homebrew packages across builds, or install fresh each time?
- **macOS code signing identity**: ad-hoc signing (`-`) or a proper Developer ID?

### Phase 6
- **Windows PHP source**: compile from source (complex) or repackage official `windows.php.net` builds (simpler, well-tested)?
- **Windows ARM64**: defer until GitHub offers ARM64 Windows runners (expected 2026–2027)?

### Phase 7
- **Auto-promotion threshold**: 50 unique workflow runs per week? Needs calibration with real data.
- **R2 mirror sync strategy**: push-based (pipeline pushes to R2 alongside GHCR) or pull-based (R2 pulls from GHCR on cache miss)?

---

## 12. Verification Strategy

### Per-phase verification

Each phase includes specific verification steps before the version tag is cut.

**Phase 1:**
1. Run `bootstrap.yml` — verify `bundles.lock` is populated
2. Trigger each of the 6 trigger types manually — verify they produce correct matrix output
3. Run integration test workflow on `ubuntu-24.04`:
   ```yaml
   - uses: buildrush/setup-php@<branch>
     with:
       php-version: '8.4'
       extensions: mbstring, intl, curl, zip, redis
   - run: php -v | grep "PHP 8.4"
   - run: php -r "foreach(['mbstring','intl','curl','zip','redis'] as \$e) assert(extension_loaded(\$e), \"\$e not loaded\");"
   ```
4. Measure cold install time — target: under 10 seconds
5. Measure warm cache hit time — target: under 2 seconds
6. Verify cosign signature on all bundles: `cosign verify ghcr.io/buildrush/php-core:8.4.x-linux-x86_64-nts`
7. Run Go unit tests: `go test -race ./...`
8. `make check` passes locally (all linters, formatters, tests, builds)
9. `ci-lint.yml` workflow passes on PRs with branch protection enforcing it
10. `make bundle-php` and `make bundle-ext` work locally via Docker
11. `release-action.yml` produces cross-compiled binaries and creates a GitHub Release for `v0.1.0-alpha`
12. `README.md` covers action usage, input reference, and contributing basics
13. `CLAUDE.md` enforces quality constraints for AI-assisted development

**Phase 2:**
1. Integration tests for PHP 8.1, 8.2, 8.3, 8.4, 8.5 on both `ubuntu-22.04` and `ubuntu-24.04`
2. Integration tests on aarch64 runner
3. Smoke tests pass for all top-50 extensions
4. Verify no regression in install time

**Phase 3:**
1. Integration test with `tools: composer:2, phpunit, phpstan`
2. Tier-3 fallback test: request an extension not in catalog, verify PECL fallback works
3. Verify warning message includes catalog PR suggestion

**Phases 5-6:**
1. Platform-specific integration tests on all target runners
2. Extension smoke tests on the new platform
3. Performance benchmarks

**Continuous verification:**
- Every PR runs the integration test suite
- Nightly workflow validates the full lockfile against GHCR
- Status pages reflect reality (lockfile matches GHCR state)
