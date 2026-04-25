# PR-gated v2 Drop-in Compat Testing — Design

**Status:** Draft — 2026-04-24
**Scope:** Close out the "diffs against goldens" step the 2026-04-23 local-CI unification spec anticipated but did not implement.

## Context

`buildrush/setup-php` is a from-scratch reimplementation of `shivammathur/setup-php`
optimized for fast, reproducible PHP setup via prebuilt OCI bundles. Its
explicit goal is **drop-in compatibility** with `shivammathur/setup-php@v2`:
same input shape (`php-version`, `extensions`, `ini-values`, `coverage`,
`ini-file`), semantically-matching output for realistic workflows.

The 2026-04-23 local-CI unification spec collapsed all prior
`compat-harness.yml`/`integration-test.yml`/`plan-and-build.yml`
orchestration into a single `phpup test` runner exercised identically
locally (`make ci-cell`) and in CI (`ci.yml::pipeline`). The
unification spec **allocated** the following pieces for drop-in compat:

- `phpup internal test-cell` iterates fixtures and runs `probe.sh`.
- Fixtures stay in `test/compat/fixtures.yaml`.
- "Goldens in `test/compat/testdata/`."
- "Diffs against goldens."

Everything except the last bullet was wired up in PR 3 of the unification
rollout (July 2026 bundle). The "diffs against goldens" step — the
actual compat gate — was never implemented. Today:

- `test-cell` runs `probe.sh` per fixture and parses the JSON, but
  compares it only against **internal invariants** (our own shape/presence
  checks); nothing in the code path ever loads a v2-baseline golden.
- `phpup compat-diff` exists as a subcommand (from the PR-6 consolidation)
  with a tested Go API (`internal/compatdiff.diffProbes`), but it is not
  called from anywhere in the pipeline.
- `docs/compat-matrix.md` carries the pinned v2 SHA
  (`accd6127cb78bee3e8082180cb391013d204ef9f`), the pinned PPA snapshot
  date (`2026-04-11`), and a populated deviations allowlist inside
  `<!-- compat-harness:deviations:start/end -->` markers.

The previously-removed `.github/workflows/compat-harness.yml` (deleted in
PR #60 of the unification rollout) ran **both** actions in every cell
and diffed their probes live. That was the right idea with the wrong
delivery shape: it coupled PR-time CI to live apt + PPA state, ran 4×N
jobs per PR, and induced cross-action PATH/PHPRC collisions. This design
keeps the original goal (a PR-time compat gate against a pinned v2
baseline) while eliminating the live-v2-install anti-pattern from every
PR.

## Goal

On every PR, detect any change that makes `buildrush/setup-php` behave
differently from `shivammathur/setup-php@v2` on a canonical set of
drop-in scenarios — without running v2 at PR time.

## Non-goals

- Running `shivammathur/setup-php` on PRs. The goldens are captured
  offline and committed.
- Matching every quirk of v2 in every axis (PHP version × arch × OS ×
  extension set). The canonical cell is one environment; broader sweeps
  are out of scope (see §10).
- A new binary or standalone workflow for compat. Compat lives inside
  `phpup internal test-cell`, run identically locally and in CI by the
  existing `make ci-cell` / `ci.yml::pipeline` path.

## Architecture

### Framing

This design is **completion of unfinished work in the 2026-04-23
local-CI-unification spec**, not a new subsystem. Every building block
exists except the wire-up from `test-cell` to `phpup compat-diff` and the
committed JSON files it reads.

```
┌──────────────────────────────────────────────────────────────────────┐
│                          PR-time flow (existing)                     │
│                                                                      │
│  ci.yml::pipeline matrix (2 OS × 2 arch × 5 PHP = 20 cells)          │
│        │                                                             │
│        └─▶ for each cell: make ci-cell OS=.. ARCH=.. PHP=..          │
│                  │                                                   │
│                  └─▶ phpup test                                      │
│                          │                                           │
│                          └─▶ docker run bare-ubuntu                  │
│                                   │                                  │
│                                   └─▶ phpup internal test-cell       │
│                                            │                         │
│                                            ├─ phpup install          │
│                                            ├─ probe.sh               │
│                                            ├─ assertInvariants       │
│                                            └─ NEW: compatDiff(...)   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

The compat-diff step is a no-op on cells where the
`(os, arch, php)` tuple ≠ `(noble, x86_64, 8.4)` or where the fixture
doesn't carry `compat: true`. Exactly one cell (noble/amd64/8.4) × eight
fixtures performs comparisons; the other nineteen cells skip compat and
exercise internal invariants only.

### Canonical cell

Goldens are captured on one environment: **Ubuntu 24.04 (noble), x86_64,
PHP 8.4**. This matches v2's canonical target (`ubuntu-latest` =
`ubuntu-24.04` on amd64) and keeps the golden count minimal.

### Golden fixtures (eight)

Axis-representative subset of `test/compat/fixtures.yaml`:

| Fixture | Axis exercised |
| --- | --- |
| `bare` | empty extensions, no ini-values, coverage: none |
| `exclusion` | `:opcache` removal token |
| `single-ext` | one shared extension (`redis`) |
| `multi-ext` | two extensions (`redis, xdebug`) |
| `none-reset` | `none, redis` — `none` hoisted, ext re-added |
| `coverage-pcov` | `coverage: pcov` auto-disable-xdebug + install |
| `ini-and-coverage` | `ini-values` merged with `coverage: xdebug` |
| `ini-file-development` | `ini-file: development` base |

All eight already exist and already run in the noble/amd64/8.4 pipeline
cell. No fixture additions are required; a new `compat: true` flag is
added to these eight entries to opt them into the gate. The flag is
absent (false) on all other fixtures.

## Components

| Component | Location | Status |
| --- | --- | --- |
| Committed goldens | `test/compat/testdata/golden/v2/<fixture>.json` | NEW — 8 files |
| Compat-diff invocation | `internal/testsuite/testcell.go:runFixture` after `assertFixtureInvariants` | MODIFY |
| Golden-presence gate | Inside `runFixture`; skip unless canonical cell AND fixture has `compat: true` AND golden file exists | NEW logic |
| Deviation artifact writer | `test-cell` writes `./deviations/<cell>.json` on compat-diff exit 1 | NEW |
| `compat-report` job | `.github/workflows/ci.yml` | NEW job, `if: failure()` |
| Golden-refresh workflow | `.github/workflows/compat-golden-refresh.yml` | NEW workflow |
| Makefile capture target | `make compat-refresh-goldens` | NEW target |
| Capture subcommand | `phpup internal golden-capture` | NEW under `internal/testsuite/` |
| Fixture `compat` field | `test/compat/fixtures.yaml` on the 8 elected entries | NEW yaml field |

No new Go packages; capture reuses `internal/testsuite/`. The in-cell
compat-diff call invokes the `compatdiff` package **directly** (Go API,
not a shell-out) for speed and error clarity. The existing internal
helper `diffProbes` is unexported; the implementation plan will either
export it, or add a narrow public wrapper, to expose it to
`testsuite`. The existing `phpup compat-diff` subcommand stays as the
CLI surface for offline debugging and does not change.

## Data flow

### PR-time (in-cell)

```
runFixture(f):
    probeJSON = probe.sh(f)
    assertInvariants(probeJSON)                           # existing
    if (cellOS, cellArch, cellPHP) == ("noble","x86_64","8.4")
            and f.Compat
            and exists("test/compat/testdata/golden/v2/"+f.Name+".json"):
        ours    = probeJSON
        theirs  = read("test/compat/testdata/golden/v2/"+f.Name+".json")
        al      = loadAllowlist("docs/compat-matrix.md")
        devs    = compatdiff.<diff-fn>(ours, theirs, al, f.Name)
        if devs non-empty:
            appendDeviationsArtifact("./deviations/noble-x86_64-8.4.json", f.Name, devs)
            fixture.Fail(devs)
```

### CI-time (Shape Z consolidation)

```
pipeline matrix:
    noble/x86_64/8.4 cell:
        - on compat fail: uploads deviations-noble-x86_64-8.4.json
        - on compat OK:   uploads nothing
compat-report (if: always() && needs.pipeline.result != 'skipped'):
    download deviations-* artifacts (may be zero)
    find existing sticky comment by <!-- compat-report-comment-v1 -->
    if deviations found:
        render markdown body (§7 template) + post-or-update sticky comment
    else if existing sticky comment present:
        rewrite its body to "✅ Compat-diff cleared" marker (prior
        red PR is now green — clear state by replacing the body, not
        by deleting the comment, so the PR's discussion retains an
        audit trail of when the gate flipped)
    else:
        no-op
```

The `compat-report` job is pull-request-scoped (gated on
`github.event_name == 'pull_request'`); it doesn't run on push-to-main
because there is no PR to comment on. Its gate is
`always() && needs.pipeline.result != 'skipped'` so it runs whether
`pipeline` passed or failed, rewriting stale deviation comments to
the cleared marker when deviations clear between pushes.

### Refresh-time (weekly + on-demand)

```
compat-golden-refresh.yml (on schedule + workflow_dispatch):
    sanity:
        assert matrix.PinnedSHA == workflow.pinnedSHA
    matrix over the 8 compat fixtures, runs-on: ubuntu-24.04:
        - uses: shivammathur/setup-php@<pinned-sha>
          with: fixture.inputs
        - bash test/compat/probe.sh → captured-<fixture>.json
        - upload artifact
    aggregate:
        download artifacts into test/compat/testdata/golden/v2/
        if git diff --quiet: exit 0 (record "no drift")
        else:
            force-push to single rolling branch `chore/compat-golden-refresh`
            open PR via peter-evans/create-pull-request (delete-branch: true)
            body = "Per-fixture key-level deltas:\n ..."
```

## Refresh workflow specifics

**File:** `.github/workflows/compat-golden-refresh.yml`

**Triggers:**
- `schedule: cron: '0 12 * * MON'` — weekly Monday 12:00 UTC.
- `workflow_dispatch:` — on-demand refresh. This is the
  escape-hatch the PR-comment template points at.

**Pinning discipline:** The workflow uses the v2 SHA literally listed in
`docs/compat-matrix.md` under `## Pinning`. A pre-run sanity step reads
the markdown-pinned SHA and refuses to run if it doesn't match the
`env.PINNED_V2_SHA` value embedded in the workflow file (belt +
braces). Bumping the pinned SHA is a deliberate manual edit to the
matrix document plus the workflow env value — not a thing the refresh
workflow does on its own.

**Refresh PR body:** generated by an aggregation step. Lists for each
fixture whose golden JSON changed: the fixture name, the diff of
top-level JSON paths (`extensions`, each `ini.*` key, `env_delta`,
`path_additions`), and whether the allowlist already tolerates those
paths. If the allowlist already covers the diff (i.e., the drift would
not have failed PR-time compat-diff), the PR body flags that explicitly
so a reviewer can decide whether to merge the refresh or leave it —
both outcomes are safe.

**Isolation:** the refresh workflow runs in its own GitHub-hosted
ephemeral runner, uses `shivammathur/setup-php@<sha>` which is the only
place in the repo that installs v2. No artifact from the refresh job
ever feeds into PR-time CI directly; the only handoff is the committed
JSON files on the drift PR's branch.

## PR comment on compat-diff failure

A sticky comment, created-or-updated by
`peter-evans/create-or-update-comment` keyed on the header
`<!-- compat-report-comment-v1 -->`. Body template:

```markdown
## ⚠️ Compat-diff detected deviations from shivammathur/setup-php@v2

<!-- compat-report-comment-v1 -->

This PR's `phpup install` behavior diverges from the pinned v2 baseline
in ways the compat-matrix allowlist doesn't cover.

### Deviations

**Fixture `multi-ext` (noble/amd64/8.4)**
- `ini.xdebug.mode`: ours=`coverage`, theirs=`develop,coverage`
- `extensions`: ours=[redis, xdebug, ...], theirs=[xdebug, ...]

<!-- one section per fixture; key lists truncated at 20 entries -->

### What to do

1. **If this is a regression in your change** — fix it, or add an entry
   to the deviations allowlist in
   [`docs/compat-matrix.md`](../blob/main/docs/compat-matrix.md) with a
   `reason:` explaining why it's intentional. The YAML block is
   delimited by `<!-- compat-harness:deviations:start -->` /
   `<!-- compat-harness:deviations:end -->`.

2. **If this looks like upstream v2 drift** (e.g., the PPA shipped a
   new package between golden refreshes) — ask a maintainer to
   force-refresh the goldens:

       gh workflow run compat-golden-refresh.yml

   Merge the resulting drift PR (`chore: refresh v2 compat goldens`),
   then rebase this PR onto `main` to pick up the new goldens.

Goldens pinned to v2 SHA `accd6127…` + PPA snapshot `2026-04-11`. Last
refreshed: <commit date of test/compat/testdata/golden/v2/>.
```

The comment is deterministic: re-running a failing PR re-posts the
same content (sticky-key match). A PR that moves from red→green on the
compat gate has its comment **replaced** with a "✅ Compat-diff
cleared" body by the `compat-report` job (it runs `if: always()` on
every pipeline completion, not only on failure; see §5 data flow),
so the PR no longer carries stale deviation text while retaining
an audit trail of the comment lifecycle.

## Failure policy

**Hard fail with an on-demand refresh escape hatch** (Shape 2 of the
brainstorm).

- Compat-diff returns exit code 1 ⇒ fixture fails ⇒ cell fails ⇒
  `pipeline` job fails ⇒ PR is red. The PR can only merge by
  either (a) removing the regression from the code, or (b) adding an
  allowlist entry to `docs/compat-matrix.md` in the same PR with a
  `reason:` (reviewable in the diff).
- Exit code 2 (malformed input or missing golden) ⇒ treat as a bug in
  the test machinery, surface with a clear error, still fail the cell.
  This prevents a silent skip if a golden is somehow deleted or
  corrupted.
- The PR comment explicitly surfaces the on-demand refresh path so a
  developer whose PR is red because of upstream v2 drift (between
  weekly scheduled refreshes) can recover without maintainer intuition:
  run the workflow, merge the drift PR, rebase.

## Testing

- **Unit tests, `internal/testsuite`:**
  - Golden-presence gate: canonical cell + non-canonical cell both run
    fixtures; only the canonical cell invokes compat-diff.
  - Compat-diff wiring: `runFixture` with stubbed probe + stubbed
    golden + the real allowlist parser → assert exit codes and
    deviation-artifact contents.
  - Deviation-artifact writer: fail twice across two fixtures, one
    artifact file collects both.
- **Unit tests, `internal/testsuite/golden_capture.go`:**
  - Uses the existing `stub-php.sh` to simulate `shivammathur/setup-php`
    output. Verifies probe shape matches what the PR-time path produces.
- **End-to-end smoke (added to `make check`):**
  - `make ci-cell OS=noble ARCH=x86_64 PHP=8.4` must show all 8 compat
    fixtures passing (`compat=OK`). If goldens are missing the cell
    must fail fast with an actionable error pointing at
    `make compat-refresh-goldens`.
  - `make ci-cell OS=jammy ARCH=aarch64 PHP=8.1` must show the fixtures
    passing with `compat=SKIP (non-canonical cell)` — compat is
    invisible to non-canonical cells.
- **Manual verification (pre-merge):**
  - Push a branch with an intentional deviation (e.g., add a random
    ini-value default), observe the `compat-report` job posts a
    correctly-formatted sticky comment.
  - Push a follow-up commit removing the deviation, observe the same
    `compat-report` job **rewrites** the sticky comment body to the
    "✅ Compat-diff cleared" marker (no stale "deviations detected"
    text left behind on a now-green PR; the comment history itself
    is preserved for audit).
  - Dispatch `compat-golden-refresh` workflow manually, observe the
    drift PR opens with reasonable body (or exits cleanly on no drift).

## Critical files

**New:**
- `.github/workflows/compat-golden-refresh.yml`
- `test/compat/testdata/golden/v2/bare.json`
- `test/compat/testdata/golden/v2/exclusion.json`
- `test/compat/testdata/golden/v2/single-ext.json`
- `test/compat/testdata/golden/v2/multi-ext.json`
- `test/compat/testdata/golden/v2/none-reset.json`
- `test/compat/testdata/golden/v2/coverage-pcov.json`
- `test/compat/testdata/golden/v2/ini-and-coverage.json`
- `test/compat/testdata/golden/v2/ini-file-development.json`
- `internal/testsuite/golden_capture.go` + `_test.go`
- `Makefile` target `compat-refresh-goldens`

**Modified:**
- `.github/workflows/ci.yml` — add `compat-report` job after `pipeline`
- `internal/testsuite/testcell.go` — compat-diff call in `runFixture`,
  deviation-artifact writer, per-fixture outcome now carries compat
  status
- `internal/testsuite/fixtures.go` — `Compat bool` field on `Fixture`
- `test/compat/fixtures.yaml` — `compat: true` on the 8 elected entries
- `cmd/phpup/main.go` — register `phpup internal golden-capture` under
  the existing `internal` dispatcher

**No deletions.**

## Verification

- **Per-fixture:** the eight committed goldens are byte-identical to
  the output of running `shivammathur/setup-php@<pinned-sha>` on
  `ubuntu-24.04` × amd64 × PHP 8.4 with the fixture's inputs, piped
  through `test/compat/probe.sh`. Reproducible by running
  `make compat-refresh-goldens` and observing no git diff.
- **Per-PR (positive):** a no-op PR (e.g., README typo) runs the full
  pipeline with the compat-diff step succeeding on all 8 fixtures.
- **Per-PR (negative):** an intentional regression (e.g., bump a
  default ini value) produces a red PR with the sticky comment
  listing the divergent keys.
- **Refresh workflow (positive):** a workflow_dispatch with no v2/PPA
  drift completes with `git diff --quiet` and records "no drift" in the
  job summary.
- **Refresh workflow (negative):** a workflow_dispatch after a
  deliberate SHA bump in `docs/compat-matrix.md` + the workflow env
  produces a drift PR with an accurate body.

## Open questions / future work

- **Broader sweep.** The 8-fixture / 1-cell PR gate explicitly does not
  cover arm64, jammy, PHP 8.1–8.3/8.5, or the extension-heavy
  `multi-ext-top10`/`multi-ext-hard4` fixtures. If arm64-only
  regressions become a pattern, expand the canonical cell set (likely
  adding `noble/arm64/8.4` as a second compat cell, capturing its own
  goldens with the `opcache.jit_buffer_size: 128M` divergence naturally
  encoded). Mechanism extends without redesign.
- **Allowlist UX.** If compat-diff false-positives become a PR-time
  annoyance (expected: rare, since the allowlist is already populated
  from the previous harness era), consider a `phpup internal compat-
  diff --propose-allowlist-entry` helper that prints a YAML snippet a
  developer can paste into the matrix.
- **Golden compression.** Eight JSON files × O(2-5KB) each = small
  enough to ignore for now. If the fixture count grows 10× the
  committed JSONs can migrate to a single consolidated
  `golden/v2.json` map without changing the compat-diff API.
