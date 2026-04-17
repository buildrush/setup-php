# Plan-and-Build Reusable Workflow — Design

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-17
**Supersedes:** Nothing (companion follow-up to the lockfile-spechash slice)
**Target release:** Next minor after `v1.2.0`

## 1. Summary

`on-push.yml` contains the full build pipeline (`plan → build-php → build-ext → update-lock`). Three other workflow files — `manual.yml`, `nightly.yml`, `security-rebuild.yml` — only call `plan.yml` and never trigger actual bundle builds. That means there is no in-repo mechanism to force a rebuild (e.g., after a builder-script change like the T12 flag additions that landed in `v1.2.0` but whose bundle has not been republished).

This slice extracts the pipeline into a new reusable workflow `plan-and-build.yml` and rewrites each of the four trigger files (`on-push.yml`, `manual.yml`, `nightly.yml`, `security-rebuild.yml`) into thin wrappers that invoke the reusable with trigger-appropriate inputs. After this lands, `gh workflow run manual.yml -f force=true` produces a real rebuild with digest publish and auto-PR.

## 2. Goals & non-goals

### Goals
- `manual.yml`, `nightly.yml`, `security-rebuild.yml` actually build bundles (not just plan).
- A single source of truth for the pipeline: plan → build-php → build-ext → update-lock.
- `manual.yml` exposes `force` and `push` inputs so an operator can do verify-only manual runs or forced republish.
- Current `on-push.yml` push/PR behavior is preserved byte-for-byte at the observable level.
- `security-rebuild.yml` actually republishes when its `repository_dispatch` webhook fires.

### Non-goals
- Changing the pipeline steps themselves (no new build stages, no fancier `update-lock`).
- Runtime behavior of `phpup`. Out of scope.
- Adding retry or failure-recovery logic. The existing `if:` gating on each job is unchanged.
- Optimizing `go run` compile overhead in `update-lock`.
- Touching `integration-test.yml`, `release-please.yml`, `ci-lint.yml`, `check-release-pr.yml`, `gc-bundles.yml`, `watch-*.yml`, `release-action.yml`, `update-major-tag.yml`, `bootstrap.yml`. These operate on different axes.
- Re-enabling `workflow_dispatch` on `on-push.yml`. After this slice, manual triggers flow through `manual.yml`.

## 3. Current state

```
on-push.yml         ── push[catalog/**,builders/**], PR[same], workflow_dispatch
  └─ plan (calls plan.yml with force=false)
  └─ build-php (matrix, push=github.event_name!=pull_request)
  └─ build-ext (matrix, push=github.event_name!=pull_request)
  └─ update-lock (if: github.event_name!=pull_request)

manual.yml          ── workflow_dispatch{force}
  └─ build (calls plan.yml with inputs.force)       ◁── dead-end, no build

nightly.yml         ── schedule 0 3 * * *, workflow_dispatch
  └─ reconcile (calls plan.yml with force=false)    ◁── dead-end

security-rebuild.yml── repository_dispatch[security-rebuild]
  └─ rebuild (calls plan.yml with force=true)       ◁── dead-end
```

`plan.yml` is already a reusable workflow (takes `force`, emits matrices as outputs). The missing piece is the build/publish chain — currently inlined only in `on-push.yml`.

## 4. Target state

```
plan-and-build.yml  ── workflow_call(force bool, push bool)
  └─ plan (force=inputs.force)
  └─ build-php (push=inputs.push)
  └─ build-ext (push=inputs.push)
  └─ update-lock (if: inputs.push)

on-push.yml         ── push[catalog/**,builders/**], PR[same]
  └─ pipeline (force=false, push=github.event_name!=pull_request)

manual.yml          ── workflow_dispatch{force bool, push bool}
  └─ pipeline (force=inputs.force, push=inputs.push)

nightly.yml         ── schedule 0 3 * * *, workflow_dispatch
  └─ pipeline (force=false, push=true)

security-rebuild.yml── repository_dispatch[security-rebuild]
  └─ pipeline (force=true, push=true)
```

`plan.yml` is unchanged. All five files above get rewritten (one new, four replaced).

## 5. Component detail

### 5.1 `plan-and-build.yml`

```yaml
name: plan-and-build
on:
  workflow_call:
    inputs:
      force:
        description: 'Force rebuild even if digests match'
        type: boolean
        default: false
      push:
        description: 'Push bundles to GHCR and update bundles.lock'
        type: boolean
        default: true

jobs:
  plan:
    uses: ./.github/workflows/plan.yml
    with:
      force: ${{ inputs.force }}

  build-php:
    needs: plan
    if: needs.plan.outputs.php_matrix != '{"include":[]}'
    strategy:
      fail-fast: false
      max-parallel: 20
      matrix: ${{ fromJSON(needs.plan.outputs.php_matrix) }}
    uses: ./.github/workflows/build-php-core.yml
    with:
      version: ${{ matrix.version }}
      os: ${{ matrix.os }}
      arch: ${{ matrix.arch }}
      ts: ${{ matrix.ts }}
      spec_hash: ${{ matrix.spec_hash }}
      push: ${{ inputs.push }}

  build-ext:
    needs: [plan, build-php]
    if: |
      always() &&
      needs.plan.result == 'success' &&
      needs.build-php.result != 'failure' &&
      needs.plan.outputs.ext_matrix != '{"include":[]}'
    strategy:
      fail-fast: false
      max-parallel: 20
      matrix: ${{ fromJSON(needs.plan.outputs.ext_matrix) }}
    uses: ./.github/workflows/build-extension.yml
    with:
      extension: ${{ matrix.extension }}
      ext_version: ${{ matrix.ext_version }}
      php_abi: ${{ matrix.php_abi }}
      os: ${{ matrix.os }}
      arch: ${{ matrix.arch }}
      spec_hash: ${{ matrix.spec_hash }}
      push: ${{ inputs.push }}

  update-lock:
    needs: [plan, build-php, build-ext]
    if: |
      always() &&
      inputs.push &&
      needs.plan.result == 'success' &&
      needs.build-php.result != 'failure' &&
      needs.build-ext.result != 'failure'
    runs-on: ubuntu-24.04
    permissions:
      contents: write
      packages: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
      - name: Update lockfile
        env:
          GHCR_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: go run ./cmd/lockfile-update -catalog ./catalog -lockfile ./bundles.lock -registry ghcr.io/${{ github.repository_owner }}
      - uses: peter-evans/create-pull-request@v8
        with:
          commit-message: "chore: update bundles.lock"
          title: "chore: update bundles.lock"
          branch: bot/lockfile-update
          delete-branch: true
          body: |
            Automated lockfile update from build pipeline.

            Run: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
```

Key change from current `on-push.yml::update-lock`: the `if:` gate switches from `github.event_name != 'pull_request'` to `inputs.push`. `github.event_name` inside a reusable callee reflects the original triggering event, which works for push/PR but is brittle; `inputs.push` is the authoritative contract.

### 5.2 `on-push.yml`

```yaml
name: on-push
on:
  push:
    branches: [main]
    paths: ['catalog/**', 'builders/**']
  pull_request:
    paths: ['catalog/**', 'builders/**']

jobs:
  pipeline:
    uses: ./.github/workflows/plan-and-build.yml
    with:
      force: false
      push: ${{ github.event_name != 'pull_request' }}
    secrets: inherit
```

Dropped `workflow_dispatch` trigger — operators use `manual.yml` instead.

### 5.3 `manual.yml`

```yaml
name: manual
on:
  workflow_dispatch:
    inputs:
      force:
        description: 'Force rebuild even if digests match'
        type: boolean
        default: false
      push:
        description: 'Push bundles to GHCR and update bundles.lock'
        type: boolean
        default: true

jobs:
  pipeline:
    uses: ./.github/workflows/plan-and-build.yml
    with:
      force: ${{ inputs.force }}
      push: ${{ inputs.push }}
    secrets: inherit
```

### 5.4 `nightly.yml`

```yaml
name: nightly
on:
  schedule:
    - cron: "0 3 * * *"
  workflow_dispatch:

jobs:
  pipeline:
    uses: ./.github/workflows/plan-and-build.yml
    with:
      force: false
      push: true
    secrets: inherit
```

### 5.5 `security-rebuild.yml`

```yaml
name: security-rebuild
on:
  repository_dispatch:
    types: [security-rebuild]

jobs:
  pipeline:
    uses: ./.github/workflows/plan-and-build.yml
    with:
      force: true
      push: true
    secrets: inherit
```

## 6. `secrets: inherit` note

The `secrets: inherit` on each caller is required so that the reusable workflow's nested calls (to `build-php-core.yml`, `build-extension.yml`) receive `${{ secrets.GITHUB_TOKEN }}` for GHCR push + cosign. The current `on-push.yml` does not pass `secrets:` explicitly — but it also doesn't need to, because the call target (`plan.yml`, `build-php-core.yml`, `build-extension.yml`) uses `${{ secrets.GITHUB_TOKEN }}` which is always available to `workflow_call`. However, since `plan-and-build.yml` is now a nested reusable (caller → `plan-and-build.yml` → `build-php-core.yml`), secrets must be explicitly inherited to reach the innermost job. Without `secrets: inherit`, `build-php-core.yml`'s `secrets.GITHUB_TOKEN` resolves to empty and `oras login` fails.

## 7. Error handling

No new error paths introduced. All existing `if:` guards on `build-ext` and `update-lock` are preserved, just rephrased in terms of `inputs.push` where appropriate.

One subtle behavior change: on a PR that edits catalog/builder files, the current `on-push.yml` runs `build-php`/`build-ext` with `push: false` (per SH-T5/T6), so no GHCR writes happen — that continues under the new shape with `pipeline.push = false`. `update-lock` is skipped on PR (its `if:` now gates on `inputs.push`). Same observable behavior.

## 8. Testing

Observable-only:
1. **PR with this refactor** — does NOT fire `on-push.yml` (path filter `.github/workflows/**` is not in the filter list). Go test coverage stays green via `integration-test.yml`. That workflow always runs and exercises the `phpup` binary, which is unaffected.
2. **Post-merge manual smoke**: `gh workflow run manual.yml -f force=true -f push=false` → planner fires with `--force`, every cell goes to `build-php` and `build-ext` with `push: false` (verify-only), artifact uploaded to workflow run, no GHCR writes, no auto-PR. Confirms the chain works.
3. **T12 bundle rebuild**: `gh workflow run manual.yml -f force=true -f push=true` → full rebuild + GHCR push + update-lock auto-PR. The new 8.4 bundle lands in `ghcr.io/buildrush/php-core:8.4-linux-x86_64-nts` with the `--with-ffi --with-gettext --enable-pcntl ...` additions compiled in. `bundles.lock` gets a fresh digest + populated spec_hash (replacing today's grandfathered empty spec_hash).
4. **Nightly resilience**: next scheduled 03:00 UTC run of `nightly.yml` confirms the reconciliation path works without an operator.

## 9. Risks

- **`secrets: inherit` forgotten**: tested by the manual run in §8.2. Easy to catch and trivial to add.
- **Reusable workflow nesting depth**: GitHub allows up to 4 levels. Today's max depth is `on-push.yml → build-php-core.yml` = 2. Under this slice: `manual.yml → plan-and-build.yml → build-php-core.yml` = 3. One level of headroom remains before the GitHub limit. Acceptable — no plans to add another layer.
- **`plan.yml` invocation as a workflow_call from a workflow_call**: that is already exercised today (`on-push.yml → plan.yml` is a nested reusable call). `plan-and-build.yml → plan.yml` is the same depth-reduction trick; works.
- **Scheduled nightly double-runs**: `nightly.yml` and any manual/push triggered run on the same day could race on `peter-evans/create-pull-request`. Mitigation: the action deletes and recreates the `bot/lockfile-update` branch idempotently. Existing behavior; unchanged.
- **`force: true` on nightly**: the design keeps nightly's `force: false`. If the team wants a weekly force-rebuild to guard against subtle drift, that's a follow-up cron entry, not a change to this design.

## 10. Delivery

- Five workflow files touched: one new (`plan-and-build.yml`), four rewritten (`on-push.yml`, `manual.yml`, `nightly.yml`, `security-rebuild.yml`).
- Conventional commit: `refactor(workflows): extract plan-and-build reusable; wire manual/nightly/security-rebuild`. Alternative, if the team wants each wrapper in its own commit for changelog readability: five commits (one per file).
- release-please interprets `refactor:` as a patch bump. If the net changelog wants a minor bump ("now `manual.yml` actually builds!") → use `feat(workflows): …` instead.
- No Go code, no Go test, no runtime binary changes. `make check` still green, but none of its Go/Node steps exercise the workflows.

## 11. Follow-ups outside this slice

- Consolidating the `plan.yml` call into `plan-and-build.yml` (eliminate `plan.yml` entirely). Not done here — `plan.yml` is cleanly reusable and nothing's broken about it.
- Making `update-lock`'s auto-PR trigger integration-test before merging. Requires coupling workflows that aren't coupled today.
- Reducing the per-run `go run ./cmd/lockfile-update` compile cost. Small enough we're ignoring it.
- Adding a weekly `force: true` nightly. Flag if drift becomes a concern.

## 12. References

- `docs/superpowers/specs/2026-04-17-lockfile-spechash-design.md` — the spec-hash slice (landed as `v1.2.0`).
- `docs/superpowers/specs/2026-04-17-phase2-t12-handoff.md` — the T12 flag additions whose bundle still needs rebuilding; this slice is the enabling infrastructure.
