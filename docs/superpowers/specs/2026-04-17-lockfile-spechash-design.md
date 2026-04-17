# Lockfile Spec-Hash & PR-Time Build Verification — Design

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-17
**Supersedes:** Nothing (companion follow-up to `2026-04-17-phase2-compat-slice-design.md`)
**Target release:** Next alpha after `v0.2.0-alpha.1`

## 1. Summary

The planner currently decides "is this bundle up to date?" by a single check: does `bundles.lock` contain an entry for the cell's canonical key (e.g. `php:8.4:linux:x86_64:nts`). Anything that changes the build inputs — `./configure` flags, the builder script, the apt dep list — slips past this check, so a change to `builders/linux/build-php.sh` or `catalog/php.yaml` does not trigger a rebuild. This document designs a spec-hash mechanism that closes that gap, and adjusts the CI pipeline so flag changes are verified at PR time rather than after merge.

Scaffolding already exists: `internal/planner.ComputeSpecHash(cell, catalogData, builderHash) string` is implemented but unwired. This slice finishes the wiring, bumps the lockfile schema, ports the bash lockfile-update script (broken by the Phase 2 compat slice's schema changes) to Go, and teaches the PR pipeline to run the build job without pushing.

## 2. Goals & non-goals

### Goals
- Planner recognizes "bundle is stale" when configure flags, builder script, or source pin change — schedules a rebuild even though the lockfile key is unchanged.
- Lockfile gains a spec-hash per entry so staleness is determinable offline without rebuilding.
- Build jobs run on PRs for verification (catch bad flags before merge) but do not push to GHCR on PR events.
- Lockfile update tooling understands the Phase-2 versioned `catalog/php.yaml` schema.
- Existing five bundles (one core + four PECL) are grandfathered: no forced rebuild on merge.

### Non-goals
- Changing OCI tag/ref conventions. Bundles stay at `ghcr.io/buildrush/php-core@<digest>`.
- Runtime awareness of spec-hash. `phpup` does not warn when running a bundle whose inputs have drifted; only the planner cares.
- Semver-aware `BuildTargets` sort — tracked separately.
- Tool-bundle planning — Phase 3.
- `gc-bundles.yml` changes. Its current behavior (prune OCI blobs no lockfile entry references) continues to work under v2 without modification.
- Automated handling of runner-image drift (Ubuntu package bumps). `watch-runner-images.yml` already covers this.

## 3. Background & current state

- `internal/lockfile` encodes v1 schema: `bundles: map[BundleKey]Digest`. `Parse` version-gates on `schema_version == 1`.
- `cmd/planner/main.go::filterExisting` drops a cell if `lockfile.Lookup(key)` has a digest AND GHCR hosts that digest. No content-of-inputs consideration.
- `internal/planner.ComputeSpecHash(cell *MatrixCell, catalogData []byte, builderHash string) string` exists but has zero callers.
- `MatrixCell` carries ABI dimensions (version, OS, arch, TS, extension metadata) but no spec-hash field.
- `.github/workflows/on-push.yml` runs the full plan/build/update-lock sequence on both pull_request and push-to-main events. `update-lock` is gated to main only via `if: github.event_name != 'pull_request'`. `build-php` and `build-ext` run on PRs today and push to GHCR — which means any PR touching the builder currently publishes artifacts before review.
- `scripts/update-lockfile.sh` uses Python YAML parsing against the old flat `abi_matrix` location at the catalog's top level. The Phase-2 compat slice moved `abi_matrix` under each version entry; the script will fail on the next run.

## 4. Architecture

### 4.1 Lockfile schema v2

`Digest` gets replaced by an `Entry` struct:

```go
type Entry struct {
    Digest   string `json:"digest"`
    SpecHash string `json:"spec_hash,omitempty"`
}

type Lockfile struct {
    SchemaVersion int                 `json:"schema_version"`
    GeneratedAt   time.Time           `json:"generated_at"`
    Bundles       map[BundleKey]Entry `json:"bundles"`
}

const currentSchemaVersion = 2
```

Migration rules:
- `Parse` accepts both the v1 string-valued form (`"key": "sha256:..."`) and the v2 struct form (`"key": {"digest": "sha256:...", "spec_hash": "..."}`), normalizing to `Entry` in memory.
- v1 entries produce `Entry{Digest: "sha256:...", SpecHash: ""}`. Empty `SpecHash` is the grandfathering signal.
- `Write` always emits v2.
- `Lookup(key) (digest string, ok bool)` keeps its existing signature for back-compat. A new `LookupEntry(key) (Entry, bool)` method exposes the full struct.
- Schema version > 2 is a hard error with a clear message ("this phpup predates the lockfile you loaded; upgrade the action").

### 4.2 Planner integration

`MatrixCell` gains a `SpecHash string` JSON-serialized field so the hash travels with the cell through GitHub Actions matrix expansion.

Two new helpers in `internal/planner`:

```go
// HashFile returns sha256:<hex> of file contents.
func HashFile(path string) (string, error)

// PerVersionYAML returns canonical YAML bytes for a single version entry,
// used as the narrow catalogData input to ComputeSpecHash.
func PerVersionYAML(spec *catalog.PHPSpec, version string) ([]byte, error)
```

(An `ExtensionYAML(spec *catalog.ExtensionSpec) []byte` mirror exists for extension cells.)

`ComputeSpecHash`'s signature is unchanged. `catalogData` is the narrow per-entity YAML (one version block or one extension entry), not the whole file, so unrelated versions do not bust each other's hash.

`cmd/planner/main.go::filterExisting` becomes:

```go
for i := range phpCells {
    cells[i].SpecHash = compute spec hash for this cell
}
// then:
entry, ok := lf.LookupEntry(key)
switch {
case !ok:
    rebuild // never built
case entry.SpecHash != "" && entry.SpecHash != cell.SpecHash:
    rebuild // inputs drifted
case !registryHas(entry.Digest):
    rebuild // bundle missing from GHCR
default:
    skip
}
```

The `entry.SpecHash != ""` guard is the grandfather rule: an empty recorded spec_hash means "we don't know what inputs produced this bundle, trust it until someone forces a rebuild."

### 4.3 Reusable workflow changes

`build-php-core.yml` and `build-extension.yml` gain two inputs:

- `spec_hash: string` — opaque, passed through to the `update-lock` job via a workflow output.
- `push: boolean` (default `true`) — when `false`, the job still compiles and packs the bundle but skips the `oras push` step. The bundle artifact is uploaded to the GitHub Actions run so it is inspectable; it does not land in GHCR.

Outputs:
- `digest: string` — unchanged for push=true; empty when push=false.
- `spec_hash: string` — echoed back for `update-lock` consumption.

### 4.4 Orchestration changes

`on-push.yml`:
- Passes `spec_hash: ${{ matrix.spec_hash }}` into each build job (the matrix cell already carries it via the planner output).
- Passes `push: ${{ github.event_name != 'pull_request' }}` to the reusable workflows. PR events run verify-only; push-to-main events run publish-enabled.
- `update-lock` already gated to non-PR events — unchanged.

### 4.5 Lockfile update tooling

New binary `cmd/lockfile-update/main.go`:
- Consumes `catalog.Catalog` via the existing `internal/catalog` package — already understands the versioned schema, so no Python YAML parsing.
- For each buildable cell, computes the current spec_hash and queries GHCR for the digest at the matching tag.
- Emits a v2 `bundles.lock` with `Entry{Digest, SpecHash}` per row.

`scripts/update-lockfile.sh` gets deleted. `on-push.yml::update-lock` calls the new Go binary instead.

### 4.6 Component layout

```
internal/lockfile/
  lockfile.go                  ← schema v2, Entry type, LookupEntry, auto-upgrade in Parse
  lockfile_test.go             ← v1→v2 migration tests, LookupEntry coverage
internal/planner/
  planner.go                   ← HashFile, PerVersionYAML, ExtensionYAML helpers
                               ← ComputeSpecHash unchanged (signature already right)
  planner_test.go              ← expand TestComputeSpecHash; add HashFile/PerVersionYAML tests
cmd/planner/main.go            ← populate cell.SpecHash; filterExisting uses LookupEntry
cmd/lockfile-update/main.go    ← NEW: replaces scripts/update-lockfile.sh
.github/workflows/
  build-php-core.yml           ← spec_hash + push inputs; spec_hash output
  build-extension.yml          ← same
  on-push.yml                  ← pass spec_hash + push through matrix
scripts/update-lockfile.sh     ← DELETED
bundles.lock                   ← migrates to v2 on first post-slice write
```

### 4.7 Data flow (flag-change example)

```
user edits catalog/php.yaml — bumps 8.4 configure_flags
    → on-push.yml pull_request trigger fires (catalog/** path filter)
    → plan job: cmd/planner/main.go runs
        → reads catalog via internal/catalog
        → for each cell: PerVersionYAML(spec, ver) + HashFile("builders/linux/build-php.sh")
                        → ComputeSpecHash → cell.SpecHash
        → filterExisting: lookfile entry's spec_hash != cell.SpecHash → rebuild
        → emits matrix with spec_hash populated on each cell
    → build-php-core.yml per matrix row, push=false (PR event)
        → compiles, packs, uploads as workflow artifact
        → no oras push
        → emits digest="" spec_hash="<the value>"
    → PR check turns green if build succeeds
    → user merges
    → push-to-main on-push.yml fires
        → same planner logic, push=true
        → build-php-core.yml pushes to GHCR, emits digest + spec_hash
    → update-lock (non-PR): cmd/lockfile-update/main.go
        → reads catalog + queries GHCR
        → writes v2 bundles.lock with {digest, spec_hash} for each entry
        → peter-evans/create-pull-request opens the auto-PR
    → auto-PR merges
    → main is in sync
```

## 5. Error handling

- Missing `builders/linux/build-php.sh` at plan time: hard error. The planner bails out rather than silently emitting cells with an empty spec_hash.
- v1 lockfile parse failure: hard error with a migration hint that names the Go binary (`cmd/lockfile-update`) as the recovery path.
- Schema version > 2: hard error naming the phpup version as the cause.
- GHCR query failure in `filterExisting`: surfaced as today; the planner errors out, which is the correct semantics (we cannot decide staleness without registry state).
- `cmd/lockfile-update` failure: the workflow step fails, `create-pull-request` does not open an auto-PR, next push retries.

## 6. Testing

### Unit
- `internal/lockfile`: v1 string-valued JSON parses into `Entry` with empty `SpecHash`; v2 struct-valued JSON parses into populated `Entry`; `Write` round-trips v2; `LookupEntry` returns the full struct; schema version > 2 errors.
- `internal/planner`: `TestComputeSpecHash` expanded with deltas per input; `TestHashFile` for the SHA-256 helper; `TestPerVersionYAML` determinism.
- `cmd/planner`: fake catalog + fake lockfile + stub OCI client; table-driven over (lockfile has key / spec_hash matches / GHCR has digest) yielding the right `filterExisting` outcome.
- `cmd/lockfile-update`: happy path against a fake registry client, asserting the emitted lockfile parses back as v2 with the expected entries.

### Integration
- End-to-end observable: post-merge, edit a configure flag on a new PR → watch `build-php` fire on the PR check. Not scripted; verified once by hand then trusted.

### Coverage
`CLAUDE.md`'s 80%-per-package floor applies. Adding the helpers should move planner and lockfile coverage up slightly; no existing coverage should regress.

## 7. Risks

- **False negatives**: spec_hash matches, rebuild would actually produce different output. Happens when an input outside our hash changes (apt package version on the runner, transitive system lib). `watch-runner-images.yml` already handles runner bumps; accepted.
- **False positives**: spec_hash mismatches after a cosmetic catalog edit. Marshaling via `yaml.Marshal(versionSpec)` keeps field order deterministic regardless of source YAML formatting, so this only happens on semantic changes. Accepted — spurious rebuild is cheap.
- **v2 lockfile on main after rollback**: if this PR merges, then gets reverted, the v2 lockfile persists. Old planners reject it. Mitigation: the rejection is a hard error with the upgrade hint from §5; no silent data loss.
- **Double-build cost on a flag-change PR**: ~30 min total (PR + main rebuild). Accepted per brainstorming Q3a.
- **`update-lock` on first post-slice merge**: produces a bundles.lock in v2 form. The compat slice's existing bundles grandfather in with empty `spec_hash`; the first flag change or forced rebuild populates them. No action needed on merge of this slice beyond the planner getting its first opportunity to compute hashes.

## 8. Delivery

- Conventional commits per the Phase 2 discipline. Expected shape:
  - `feat(lockfile): schema v2 with per-entry spec_hash`
  - `feat(planner): wire ComputeSpecHash into filterExisting`
  - `feat(workflows): carry spec_hash and gate push on PR events`
  - `feat(cmd): port update-lockfile to Go against versioned catalog`
  - `chore: remove scripts/update-lockfile.sh`
- No AI attribution. `make check` must pass before every commit.
- release-please cuts the next alpha (`v0.2.0-alpha.2` given the compat slice took `.1`).
- Post-merge follow-up: open a one-line PR that runs `manual.yml` with `force=true` for the existing 5 bundles if the team wants them to carry a populated `spec_hash` from day one. Optional; grandfathering works fine without.

## 9. References

- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` — parent Phase 2 compat slice.
- `docs/superpowers/specs/2026-04-17-phase2-t12-handoff.md` — T12 builder flag diff (the case this design handles automatically).
- `docs/superpowers/plans/2026-04-17-phase2-compat-slice.md` — compat slice implementation plan (contains Task 11 for the `catalog/php.yaml` schema that broke `update-lockfile.sh`).
- `internal/planner/planner.go::ComputeSpecHash` — existing scaffold.
- `CLAUDE.md` — quality gates, commit discipline, coverage target.
