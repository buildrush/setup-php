# smoke-action PR self-containment

**Issue:** [#83](https://github.com/buildrush/setup-php/issues/83)
**Status:** Design approved 2026-04-27
**Triggered by:** PR #76 (redis 6.3.0) deadlocked on `smoke-action` because it resolves bundles via the committed `bundles.lock` + GHCR, and `publish` is gated on `smoke-action` succeeding — so version-bumping PRs deadlock the lockfile-update flow on main.

## 1. Goal & scope

Make `smoke-action` consume the OCI layouts the `pipeline` job builds in the same CI run, so PRs that change bundle content (PECL version bumps, configure-flag changes, schema-version bumps) can validate the public action surface end-to-end without depending on GHCR.

This realigns the implementation with the PR self-containment design from `docs/superpowers/specs/2026-04-23-local-ci-unification-design.md` ("No cross-job GHCR roundtrip — A pipeline cell builds PHP-core → builds extensions → runs fixtures against the just-built artifacts in *its own* OCI layout. GHCR is touched only by the `publish` job after the full matrix passes on `main`.") and `2026-04-20-bundle-schema-and-rollout-design.md` ("PR CI produces a self-consistent artifact set: every runtime assertion the PR introduces is validated against a bundle that the PR itself published.").

### In scope

- Drop the `if: github.ref == 'refs/heads/main'` gate on `pipeline.Upload oci-layout artifact` so PR runs upload per-cell layouts.
- New `phpup internal lockfile-from-layout` subcommand wrapping the existing `internal/testsuite/runner.go:writeLayoutLockfileOverride` logic.
- `smoke-action` downloads the matching cell's oci-layout artifact, synthesizes a lockfile override, sets `PHPUP_REGISTRY=oci-layout:<path>` and `PHPUP_LOCKFILE=<override>` for the `uses: ./` action invocation.

### Out of scope

- Promoting PR-local bundles to GHCR. The `publish` job remains main-only — that's intended.
- Changing the `smoke-action` matrix shape or `pipeline` test execution (already self-contained).
- Re-landing redis 6.3.0 — separate slice that resumes once this lands and unblocks the lockfile flow.

## 2. Architecture

Three components, one cohesive change.

### 2.1 Artifact upload on PRs (CI workflow)

**File:** `.github/workflows/ci.yml`

Drop the conditional on the artifact upload step. The `actions/upload-artifact@v7` step keeps `retention-days: 7` so PR storage cost is bounded. Each pipeline cell uploads `oci-layout-<os>-<arch>-<php>` regardless of branch.

```yaml
- name: Upload oci-layout artifact
  uses: actions/upload-artifact@v7
  with:
    name: oci-layout-${{ matrix.os }}-${{ matrix.arch }}-${{ matrix.php }}
    path: out/oci-layout
    retention-days: 7
```

(Was: `if: github.ref == 'refs/heads/main'` — removed.)

### 2.2 `phpup internal lockfile-from-layout` subcommand (Go)

**File:** new file `internal/lockfilefromlayout/lockfilefromlayout.go`. Hooks into the existing `phpup internal <subcmd>` dispatcher in `cmd/phpup/main.go` (currently routes to `testsuite.InternalMain`).

The subcommand is a thin wrapper over `internal/testsuite/runner.go:writeLayoutLockfileOverride`. To avoid coupling the testsuite package to the new subcommand, factor the logic into a shared package: extract `writeLayoutLockfileOverride` into a new exported function in a new package `internal/layoutlockfile`, leave a one-line shim in testsuite that calls it. The new subcommand is then a 30-line CLI wrapper.

```
phpup internal lockfile-from-layout \
  --layout oci-layout:./out/merged-layout \
  --out ./bundles-override.lock
```

Behavior: walks the layout's annotated manifests, maps each `io.buildrush.bundle.key` annotation to its digest + spec_hash, writes a v2 lockfile JSON to `--out`. Identical schema to the existing override the testsuite synthesizes — same field shapes, same `schema_version: 2`.

Failure modes are explicit: missing layout → `layout: open: <err>`, no manifests → empty lockfile (not an error — caller decides). No GHCR network calls.

### 2.3 `smoke-action` consumes the artifact (CI workflow)

**File:** `.github/workflows/ci.yml`

Add four steps before `Invoke action (uses ./)`:

1. `actions/download-artifact@v8` for `oci-layout-${{ matrix.os }}-${{ matrix.arch }}-8.4` into `out/oci-layout`.
2. Build phpup (already happening in the existing "Build phpup from PR HEAD" step — reuse its output).
3. Run `phpup internal lockfile-from-layout --layout oci-layout:$PWD/out/oci-layout --out $RUNNER_TEMP/bundles-override.lock`.
4. Set step-level `env` on the `uses: ./` step: `PHPUP_REGISTRY: oci-layout:$PWD/out/oci-layout` and `PHPUP_LOCKFILE: $RUNNER_TEMP/bundles-override.lock`.

The `uses: ./` step then runs the action wrapper. `src/index.js` invokes phpup, which honors both env overrides (already implemented at `cmd/phpup/main.go:53` and `:195`).

`smoke-action` already has `needs: pipeline`, so the artifact is guaranteed to exist by the time `smoke-action` cells start.

### 2.4 No changes elsewhere

- `pipeline` test execution: already self-contained, no change.
- `publish` job: still gated on `main`. The smoke pass on PRs comes from the PR-local layout; the eventual GHCR push still happens only post-merge.
- `bundles.lock` semantics: unchanged. PR runs do not commit a lockfile update to the PR head.

## 3. Verification

Per CLAUDE.md "CI failures: reproduce locally first" — every step verifiable on a developer machine before pushing.

### 3.1 Unit-level

- `internal/layoutlockfile`: a new test asserts the function emits a valid v2 lockfile from a synthetic layout containing two annotated manifests. Mirrors the style of existing testsuite tests.
- `cmd/phpup`: a new test for the `phpup internal lockfile-from-layout` CLI wiring (flag parsing, exit codes for missing flags, integration through to a temp `t.TempDir()` layout).
- Existing `internal/testsuite` tests: unchanged behavior (the shim must produce identical output to the previous inline path).

### 3.2 Local CI cell

`make ci-cell OS=jammy ARCH=x86_64 PHP=8.5` — the existing local repro covers `pipeline` and the test container's lockfile synthesis (which uses the same logic). After the refactor, this must still pass.

### 3.3 PR CI

The PR validates itself: the `smoke-action` cells exercise the new code path. If the PR's CI shows `smoke-action` green for jammy×amd64 / jammy×arm64 / noble×amd64 / noble×arm64 against PHP 8.4 + `redis, intl`, the fix works end-to-end. (Note: this is the first PR where the new logic actually runs against a redis 6.3.0 in-cell build. It's the only valid integration test.)

### 3.4 Post-merge

On merge to main, the same `smoke-action` cells exercise the new path. `publish` is no longer blocked, so the lockfile auto-commit + GHCR push happen. The redis 6.3.0 bundles land on GHCR; the next time anyone uses `@main`, redis works.

## 4. Risk & rollback

### 4.1 Risks

- **Artifact storage cost.** ~20 layouts × N MB each per PR run, retention 7d. Bounded; existing main-only behavior was already producing one set per main push.
- **Layout-vs-GHCR digest divergence.** The synthesized lockfile uses local-layout digests, which differ from post-publish GHCR digests because `meta.json` carries a build-time timestamp. The testsuite already handles this — same logic.
- **download-artifact name mismatch.** Smoke matrix is `(os, arch)` × PHP 8.4; pipeline matrix is `(os, arch, php)`. Smoke's download must match `oci-layout-<os>-<arch>-8.4` exactly. A typo here surfaces as the same "not found in lockfile" error we're trying to fix — visible immediately in PR CI.
- **smoke-action arch normalization.** Smoke uses `amd64`/`arm64`; pipeline uses `x86_64`/`aarch64`. The artifact name must match what pipeline uploaded, so smoke needs to translate (`amd64` → `x86_64`, `arm64` → `aarch64`) before constructing the artifact name.

### 4.2 Rollback

Revert the PR. Pre-PR behavior restores: PRs upload no artifacts, smoke-action queries GHCR + committed lockfile. The deadlock returns, but no regression beyond pre-PR state.

## 5. Success criteria

- `phpup internal lockfile-from-layout --layout ... --out ...` produces a valid v2 lockfile from an oci-layout populated by `phpup build cell`.
- `make check` passes; new tests pass.
- PR CI: `pipeline` 20/20 + `smoke-action` 4/4 green in the same PR run.
- Post-merge to main: `publish` runs, pushes redis 6.3.0 bundles to GHCR, commits `chore(lock): update bundles.lock from pipeline <id>` back to main.
- Subsequent bundle-changing PRs (e.g., the eventual igbinary 3.2.17 bump in #43) pass `smoke-action` without manual intervention.
