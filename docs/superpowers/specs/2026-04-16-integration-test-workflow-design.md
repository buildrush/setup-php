# Integration Test Workflow Design

## Context

Phase 1 of `buildrush/setup-php` has the build pipeline, catalog, planner, and action entry point in place. `ci-lint.yml` gates PRs for code quality (Go lint, Node lint, tests). The build workflows (`build-php-core.yml`, `build-extension.yml`) verify that bundles compile and pass smoke tests.

What's missing: nothing tests the action itself end-to-end. The Node.js bootstrap (`src/index.js`), the Go binary's input resolution, OCI fetch, extraction, composition, and environment export — none of this is exercised in CI. A regression in any of these would only surface when a consumer uses the action.

This spec adds a dogfood workflow that builds the `phpup` binary from the current branch and invokes the action via `uses: ./` across a matrix covering the full Phase 1 catalog.

## Workflow: `.github/workflows/integration-test.yml`

### Trigger

```yaml
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
```

Matches `ci-lint.yml`. Every PR is gated; every merge to main is verified.

### Job 1: `build`

Runs on `ubuntu-24.04`. Produces the `phpup` binary as a workflow artifact.

Steps:
1. `actions/checkout@v4`
2. `actions/setup-go@v5` with `go-version: '1.26'`
3. Copy `bundles.lock` into `cmd/phpup/` (required by Go embed)
4. `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o phpup ./cmd/phpup`
5. Remove the copied `bundles.lock` from `cmd/phpup/`
6. `actions/upload-artifact@v4` — upload `phpup` binary

### Job 2: `test` (matrix)

Depends on `build`. Runs on `ubuntu-24.04`.

#### Matrix

Uses an explicit `include` list (not a cross-product) so each cell has its own extension, ini-values, and verification expectations:

| name | extensions | ini-values | verify_extensions | verify_ini |
|------|-----------|------------|-------------------|------------|
| `bare` | *(empty)* | *(empty)* | `mbstring curl intl zip openssl json pdo sodium gd opcache` | *(none)* |
| `redis` | `redis` | *(empty)* | `redis` (plus the bundled set) | *(none)* |
| `bundled` | `mbstring, intl, curl` | *(empty)* | `mbstring intl curl` (plus the bundled set) | *(none)* |
| `ini` | *(empty)* | `memory_limit=512M` | bundled set | `memory_limit => 512M` |

#### Steps

1. `actions/checkout@v4`
2. `actions/download-artifact@v4` — download the `phpup` binary
3. Place binary in tool cache:
   ```bash
   mkdir -p "$RUNNER_TOOL_CACHE/buildrush-bin"
   cp phpup "$RUNNER_TOOL_CACHE/buildrush-bin/phpup"
   chmod +x "$RUNNER_TOOL_CACHE/buildrush-bin/phpup"
   ```
4. Invoke the action:
   ```yaml
   - uses: ./
     id: setup
     with:
       php-version: '8.4'
       extensions: ${{ matrix.extensions }}
       ini-values: ${{ matrix.ini-values }}
   ```
5. Verify version:
   ```bash
   php -v
   php -v | grep -q "PHP 8.4"
   ```
6. Verify extensions via `php -i`:
   ```bash
   for ext in $VERIFY_EXTENSIONS; do
     php -i | grep -qi "${ext}" || { echo "FAIL: ${ext} not loaded"; exit 1; }
   done
   ```
   For redis specifically, also run: `php -r 'assert(extension_loaded("redis"));'`
7. Verify ini-values (ini cell only):
   ```bash
   if [ -n "$VERIFY_INI" ]; then
     php -i | grep -q "$VERIFY_INI" || { echo "FAIL: ini value not set"; exit 1; }
   fi
   ```
8. Verify action output:
   ```bash
   echo "Resolved version: ${{ steps.setup.outputs.php-version }}"
   test -n "${{ steps.setup.outputs.php-version }}"
   [[ "${{ steps.setup.outputs.php-version }}" == 8.4* ]]
   ```

### Permissions

```yaml
permissions:
  contents: read
  packages: read
```

Read-only. The test job pulls bundles from GHCR (public, but token-authenticated for rate limits).

## What This Tests End-to-End

- `src/index.js` — Node.js bootstrap (tool cache lookup, binary execution)
- `cmd/phpup` — input resolution, lockfile lookup, OCI client, extraction, composition
- `internal/env` — GitHub Actions environment variable parsing
- `internal/oci` — bundle fetch from GHCR
- `internal/extract` — zstd decompression and tar extraction
- `internal/compose` — PHP + extension layering
- Extension install path — redis PECL extension overlay
- INI configuration — custom php.ini values
- Action outputs — `php-version` output is set correctly

## What This Does NOT Test

- The binary download path in `index.js` (bypassed by pre-placing in tool cache)
- macOS / Windows / ARM64 (not in Phase 1 catalog)
- PHP versions other than 8.4 (not in Phase 1 catalog)
- Coverage drivers (xdebug/pcov bundles don't exist yet)
- Tools input (no tool bundles yet)

## Files to Create/Modify

| File | Action |
|------|--------|
| `.github/workflows/integration-test.yml` | Create — the new workflow |

No other files need modification. The action, binary, and bundles already exist.
