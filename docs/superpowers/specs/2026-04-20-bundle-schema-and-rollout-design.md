# Bundle Schema and Rollout — Eliminating "Bundle Flag Day"

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-20
**Supersedes:** Nothing — cross-cutting; lands before Phase 2 slice #4
**Target release:** `v0.2.0-alpha.3`

## 1. Summary

PR #28 (Phase 2 compat closeout) exposed a class of failure the phased design does not currently handle: a slice that changes what a bundle **contains** (layout, file, kind, platform) lands runtime assertions *before* any published bundle satisfies those assertions. The result in #28 was three debugging commits against the PR branch — a manual `manual.yml` dispatch to rebuild the 8.4 core, a cherry-picked lockfile update, and runtime-wiring fixes (`PHPRC`, opcache loader, exclusion-aware compose, residual allowlist entries) — several rounds of intervention on what should have been a straightforward merge. This class of problem recurs on every remaining slice that changes the bundle: top-50 extensions (P2 #4), tools (P3, new `php-tool` kind), macOS/Windows (P5/P6, per-platform schemas), R2 fallback (P7).

Root cause: `plan-and-build.yml` gates `update-lock` on `inputs.push`, which `on-push.yml` sets to `false` for `pull_request` events. PR CI therefore runs the new runtime against the pre-slice lockfile, and the fresh lockfile catches up via a **bot PR against main** *after* the feature PR merges. Between those two merges main is inconsistent, and the PR itself never exercises the bundle it actually needs.

This spec adopts **PR self-publishing with the lockfile committed to the PR head ref** as the structural fix, plus a **`schema_version` field on the bundle sidecar** as a diagnostics seam. Under the new flow:

- PR CI builds any bundles whose spec differs from `bundles.lock` on the PR branch, pushes them to GHCR (content-addressed, immutable), writes the updated `bundles.lock` **back to the PR head ref**, and then runs the compat harness and integration tests against the just-published bundles.
- The PR is atomic: code, bundle content, and lockfile travel together. Merging lands all three on main in a single commit.
- A declined PR leaves no trace on main and no trace on any release. Orphan GHCR digests are reaped by `gc-bundles.yml`, whose logic is fleshed out here rather than being deferred to Phase 7.
- The runtime carries a minimum `schema_version` expectation; future slices bump the sidecar version when the bundle shape changes, converting implicit "file not found" errors into explicit "needs schema ≥ N" messages.

The result eliminates the flag-day class of problem for every remaining phase without regressing the product-vision §16 reproducibility guarantee (pinned release versions still resolve byte-identical installs via their embedded lockfile).

## 2. Goals & non-goals

### Goals

- PR CI produces a self-consistent artifact set: every runtime assertion the PR introduces is validated against a bundle that the PR itself published.
- The feature PR branch is the only ref ever written to during PR CI. Main and release tags are never mutated by PR-scoped pipelines.
- Declined PRs leave no lockfile change on main and no reachable reference from any released binary; the branch's GHCR digests become orphans and are garbage-collected on a bounded schedule.
- `bundles.lock` updates move from a bot-PR-against-main workflow to an in-branch commit, atomically merged with the code that requires it.
- Bundles carry a numeric `schema_version` in their sidecar metadata; the Go runtime asserts a minimum version before composing the bundle. Version mismatches produce a structured diagnostic, not a path-not-found.
- The `gc-bundles.yml` stub is replaced with a working retention job that is branch-aware: digests referenced by `bundles.lock` on **any** live ref (main, any open branch, any release tag) are preserved; everything else is eligible for pruning after a configurable age.
- Fork PRs are handled with a clear-failure path (no partial publish), not a silent break.

### Non-goals

- Changing the OCI artifact type, push mechanism (`oras`), or content-addressing scheme. The digest formula in product-vision §6.2 is unchanged.
- Changing the action release flow. `release-please.yml` continues to cut releases from main; released binaries continue to `go:embed` main's `bundles.lock`.
- Cosign signature strategy (Phase 7 hardening).
- R2 fallback mirror (Phase 7).
- Any runtime-behavior change other than the `schema_version` assertion.
- PR-based publishing of **action releases** — this spec only covers **bundle** publishing. Action tags remain exclusively main-sourced.
- Reworking the compat harness itself (covered by `2026-04-20-compat-harness-design.md`).

## 3. Source of truth

| Field | Value |
|---|---|
| Exhibit A | PR #28 (merged 2026-04-20), commits `b911aa5`, `fe72372`, `9c33258` |
| Today's workflows referenced | `.github/workflows/on-push.yml`, `plan-and-build.yml`, `gc-bundles.yml` |
| Lockfile schema version | Repo `bundles.lock` currently `schema_version: 2` (lockfile-level, distinct from per-bundle schema introduced here) |
| Existing `cmd/lockfile-update` | Invoked by `plan-and-build.yml` at line 84 — repurposed for in-branch commits |
| Sidecar today | `builders/common/pack-bundle.sh` writes `meta.json` with `{build_timestamp, digest, builder_versions}` — extended here |

## 4. Architecture

### 4.1 Component touchpoints

| Layer | Change |
|---|---|
| `.github/workflows/on-push.yml` | `push: true` for both `pull_request` and `push` events; fork detection short-circuits the publish steps with an actionable `::warning::`. |
| `.github/workflows/plan-and-build.yml` | `update-lock` job runs whenever `inputs.push` is true (as today) but commits to the triggering ref (PR head or main), not to a bot branch. The `peter-evans/create-pull-request` step is replaced by a direct `git push` to `${{ github.head_ref \|\| github.ref_name }}`. New job `harness-and-integration` runs after `update-lock` against the freshly written lockfile. |
| `.github/workflows/compat-harness.yml` | Triggered as a `needs: update-lock` dependency within `plan-and-build.yml` — the harness runs **after** the lockfile commit is on the PR head, so the "ours" side resolves against the freshly published bundles. |
| `cmd/lockfile-update` | Add `--commit` flag that commits the modified lockfile in place and pushes to `HEAD`. The existing "write to working tree" mode stays for local-dev use. Commit message is templated (`chore(lock): update bundles.lock from pipeline <run-id>`). |
| `builders/common/pack-bundle.sh` | Emit `schema_version` field in `meta.json`. Version source: `builders/common/bundle-schema-version.env` (single-variable env file, grepped from shell and Go). |
| `cmd/planner` / `internal/planner` | Include the relevant `BUNDLE_SCHEMA_*` literal from `bundle-schema-version.env` in each bundle kind's spec-hash input. Without this, bumping the env file does not trigger a rebuild and the rollout silently regresses. |
| `internal/oci` (or new `internal/sidecar`) | Parse `meta.json` on fetch; expose `Bundle.SchemaVersion()`. |
| `internal/compose` | Before composing, assert `bundle.SchemaVersion >= compose.MinSchemaVersion(bundleKind)`. Mismatch → hard error with the expected/actual pair and a link to the schema changelog. |
| `internal/version` | Export `MinBundleSchema(kind)` — compose consults it. Mapping is compiled into the binary. |
| `.github/workflows/gc-bundles.yml` | Replace the stub: iterate GHCR packages, union the referenced digests across all live refs' `bundles.lock`, plus every released tag's embedded lockfile; prune unreferenced digests older than `GC_MIN_AGE_DAYS` (default 30). |
| `docs/product-vision.md` §6.1, §6.2, §11 | Document the `schema_version` sidecar field and the "PR self-publishes" invariant. |
| `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` | Add a one-paragraph note to Phase 2 introducing the new rollout invariant; Phase 7 GC section references this spec as the source. |
| `CONTRIBUTING.md` | New "CI writes to your PR branch" note: avoid force-push while CI is in flight; rebases are safe after a green run. |

Unchanged: `cmd/phpup` flow, `internal/plan`, `internal/resolve`, `internal/catalog`, `cmd/planner`, `cmd/compat-diff`, `internal/compat`, release-please configuration.

### 4.2 The PR-self-publish flow (chosen option)

Two options were considered for eliminating flag day:

- **C-sharp — PR pushes bundles, commits lockfile to the PR head.** Chosen. Satisfies the "no remainder on main, no side effect on releases" constraint: main only observes the lockfile at merge time; declined PRs decay to GC.
- **A — Degrade-by-default in runtime.** Rejected. Intentionally masking the mismatch makes CI go green while the real defect remains; the whole point of the harness is that this layer is supposed to fail loudly.

Mechanics under C-sharp, in order per PR push:

1. **Plan.** `plan.yml` diffs `catalog/` against the PR branch's `bundles.lock`. Cells whose spec-hash already has a live digest on GHCR are skipped. Everything else is queued.
2. **Build & push.** `build-php-core.yml` and `build-extension.yml` run per matrix cell. Pushes to GHCR are content-addressed — idempotent if another ref already pushed the same digest.
3. **Lockfile commit-back.** `update-lock` invokes `cmd/lockfile-update --commit` with the new digests; the tool writes `bundles.lock`, runs `git add bundles.lock`, commits with the templated message, and pushes to `HEAD`. On fork PRs, this step is skipped with `::warning::` and the whole pipeline exits non-zero so the PR is visibly blocked until a maintainer labels it.
4. **Harness and integration.** `compat-harness.yml` and integration tests run **after** step 3 so they resolve against the new lockfile. If they fail, the PR is red; the bundles are already on GHCR but orphan (not referenced by a non-PR ref). GC eventually reaps them.
5. **Merge.** Main adopts the PR head's commits as-is. The lockfile commit is part of the merge history.
6. **Decline.** Branch deleted ⇒ lockfile never reaches main ⇒ orphan digests → pruned by `gc-bundles.yml`.

Concurrency: two PRs that modify the same spec-hash produce the same digest (content addressing) — no conflict at the artifact layer. Lockfile conflicts on `bundles.lock` are semantic and resolved at rebase time exactly like any other file conflict.

Force-push: a contributor force-pushing during CI may race with the `update-lock` git push. Mitigation: the lockfile commit step uses `git push --force-with-lease`, emits a `::warning::` on lease failure, and the pipeline fails the PR rather than overwriting. CONTRIBUTING.md documents "wait for CI before rebasing."

### 4.3 Fork PR handling

Fork PRs cannot receive `contents: write` on their head ref and cannot use the package-write `GITHUB_TOKEN` — both are GitHub-platform constraints, not policy choices. The `update-lock` job detects fork context via `github.event.pull_request.head.repo.full_name != github.repository` and:

- Skips the bundle push and lockfile-commit steps.
- Emits a single `::warning::` explaining that fork PRs that change bundle contents must be re-run by a maintainer after a `safe-to-build` label is applied.
- Fails the pipeline (non-zero exit), so the PR shows a visible red check and is not mergeable until the label/re-run cycle.

A follow-up workflow (`fork-rebuild.yml`, out of scope here) can pick up labeled fork PRs and run the same pipeline via `pull_request_target` under a trusted-contributor gate. That is deferred to Phase 7.

Same-repo branch PRs from collaborators are the expected path through Phases 2–6 and the default the spec optimizes for.

### 4.4 GC policy (branch-aware)

`gc-bundles.yml` evolves from stub to implementation. Trigger: quarterly cron (current) plus `workflow_dispatch`.

Retention rule, evaluated per GHCR package digest:

- **Keep** if referenced by `bundles.lock` on `main` **or** any non-deleted remote branch **or** any release tag's embedded lockfile.
- **Prune** if unreferenced and the manifest's creation timestamp is older than `GC_MIN_AGE_DAYS` (default 30, env-configurable).
- **Never prune** digests referenced by any published release — enforced by iterating `gh release list --json tagName` and downloading each release's `bundles.lock` asset (or falling back to `git show <tag>:bundles.lock`).

Reference enumeration:

```
referenced := union(
    parse(refs/heads/main:bundles.lock),
    for branch in remote branches: parse(branch:bundles.lock),
    for tag in releases:            parse(tag:bundles.lock)
)
```

Performance: branch count is bounded by `git ls-remote`; at steady state (<100 open branches) this is a few seconds.

Security: GC is guarded by `permissions: packages: write, contents: read`. No token is exposed to untrusted code.

### 4.5 Release isolation invariants

These invariants must hold after this spec lands and must be asserted by CI where feasible:

1. **Releases embed main's lockfile only.** `release-please.yml` runs exclusively on main and `go:embed`s the main-tree `bundles.lock` into the binary. Unchanged.
2. **No PR-branch bundle is referenced by any released lockfile.** Enforced by construction: releases cut from main, main's lockfile only updated via merged PRs.
3. **GC respects released lockfiles.** §4.4 rule.
4. **Reproducibility preserved** (product-vision §16.3): a pinned `@vX.Y.Z` resolves to its embedded lockfile digests, which remain on GHCR as long as any release references them.

A `test/invariants/release-lockfile-immutability.go` test (added as part of this slice's testing) walks released-tag lockfiles and asserts every referenced digest is reachable on GHCR.

### 4.6 The `schema_version` sidecar field

Today's `meta.json` produced by `builders/common/pack-bundle.sh`:

```json
{
  "build_timestamp": "2026-04-20T13:14:32Z",
  "digest": "sha256:aa58…",
  "builder_versions": { "gcc": "…", "autoconf": "…" }
}
```

Extended shape:

```json
{
  "schema_version": 2,
  "kind": "php-core",
  "build_timestamp": "2026-04-20T13:14:32Z",
  "digest": "sha256:aa58…",
  "builder_versions": { "gcc": "…", "autoconf": "…" }
}
```

`schema_version` is an integer; `kind` is `php-core | php-ext | php-tool` (new field, primary use is future tooling — for now it also lets the runtime pick the right `MinBundleSchema` lookup). The version source is a single tracked file, `builders/common/bundle-schema-version.env`, so the shell builders and Go runtime read the same literal:

```sh
# builders/common/bundle-schema-version.env
BUNDLE_SCHEMA_PHP_CORE=2
BUNDLE_SCHEMA_PHP_EXT=1
BUNDLE_SCHEMA_PHP_TOOL=1
```

Initial values:

- `php-core` = **2** — bumped by this slice, because Phase 2 compat closeout (PR #28) introduced `share/php/ini/*` as a required path.
- `php-ext` = **1** — unchanged.
- `php-tool` = **1** — introduced when Phase 3 lands the kind.

Go side: `internal/version` exports `MinBundleSchema(kind string) int` returning the minimum the current runtime requires. Compose checks `bundle.SchemaVersion >= MinBundleSchema(bundle.Kind)` per bundle; mismatch is a hard error:

```
bundle php-core@sha256:aa58… has schema_version=1; runtime requires ≥ 2
(the bundle pre-dates share/php/ini/ stashing; rebuild via on-push.yml
or see docs/bundle-schema-changelog.md).
```

A new `docs/bundle-schema-changelog.md` tracks bumps — one-line entry per bump describing what changed in the bundle shape and the commit that introduced the runtime requirement.

### 4.7 When to bump `schema_version`

Bump the relevant kind's version when **any** of the following changes in a slice that modifies bundles:

- Addition/removal of a file or directory the runtime asserts on.
- Change in on-disk layout (path rename, symlink introduction).
- Change in the compose contract (e.g. opcache auto-load handling from PR #28 would qualify if it depended on new bundle-side content).
- New bundle kind: the kind starts at `schema_version = 1` and is added to `MinBundleSchema`.

Do **not** bump for:

- Upstream-source-only changes (new PHP patch version, new extension release with identical layout).
- Builder-tool version changes.
- Cosmetic metadata additions that the runtime does not assert on.

### 4.8 Flag-day recurrence check

Each slice that bumps `schema_version` must add a **single line** to the spec's "Exit criteria" section: *"After merge, `gc-bundles.yml` dry-run shows zero unreferenced digests that were reachable from main before this slice."* This protects against accidental orphaning of pre-bump bundles that are still referenced by released tags.

## 5. Workflow mechanics

### 5.1 `on-push.yml` — minimal diff

```yaml
# before
push: ${{ github.event_name != 'pull_request' }}
# after
push: true
```

The `pipeline` job already has `contents: write`. Nothing else changes.

### 5.2 `plan-and-build.yml` — `update-lock` rewrite

The current step uses `peter-evans/create-pull-request`. Replace with a direct-commit step that writes to `HEAD`:

```yaml
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
  steps:
    - uses: actions/checkout@v6
      with:
        ref: ${{ github.head_ref || github.ref_name }}
        # token default is sufficient; fork-PR detection below
    - name: Refuse fork PRs
      if: github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name != github.repository
      run: |
        echo "::warning::Fork PRs cannot auto-publish bundles. Ask a maintainer to label with safe-to-build and re-run."
        exit 1
    - uses: actions/setup-go@v6
      with: { go-version: '1.26' }
    - name: Update and commit lockfile
      run: |
        go run ./cmd/lockfile-update \
          --catalog ./catalog \
          --lockfile ./bundles.lock \
          --registry ghcr.io/${{ github.repository_owner }} \
          --commit
```

The `--commit` flag of `cmd/lockfile-update` does: write file, `git add`, `git diff --quiet` early-exit when unchanged, otherwise `git commit -m "…"`, `git push --force-with-lease origin HEAD:${{ github.head_ref || github.ref_name }}`.

A new downstream job `harness-and-integration` takes `needs: update-lock` and runs the compat harness + integration workflow against the freshly committed lockfile.

### 5.3 `cmd/lockfile-update` — `--commit` semantics

Today the tool only writes the file. Adding:

- `--commit` — if set, run `git add ./bundles.lock`, exit 0 if no diff, otherwise `git commit` with a templated message (`chore(lock): update bundles.lock from pipeline <run-id>`), then `git push --force-with-lease origin HEAD:<branch>`. The branch name is read from `GITHUB_HEAD_REF` or `GITHUB_REF_NAME` env vars (the workflow sets these implicitly).
- Git identity comes from the `GITHUB_ACTOR`-derived bot (existing convention matching `peter-evans/create-pull-request` defaults).

The non-commit mode remains for local-dev use (`make update-lock`).

### 5.4 `gc-bundles.yml` — real implementation

Behavior in pseudo-code form (actual implementation goes into `cmd/gc-bundles`):

```
references := set()
references |= parse_lockfile("refs/heads/main:bundles.lock")
for branch in git_ls_remote_branches():
    references |= try_parse_lockfile("refs/heads/" + branch + ":bundles.lock")
for tag in gh_release_list():
    references |= try_parse_lockfile("refs/tags/" + tag + ":bundles.lock")

for pkg in ghcr_list_packages("ghcr.io/buildrush"):
    for digest in pkg.manifests:
        if digest not in references and age(digest) > GC_MIN_AGE_DAYS:
            ghcr_delete(pkg, digest)
```

Dry-run mode (default in CI, real-delete on `workflow_dispatch` with `confirm: true` input) for a reviewable surface before each actual prune.

## 6. Testing

### 6.1 Unit tests

- `internal/oci` (or `internal/sidecar`): parse `meta.json` including the new fields; fail gracefully on older sidecars (`schema_version` missing → treat as 1, preserving pre-slice bundles until the first rebuild).
- `internal/version`: `MinBundleSchema` table tests — one per known kind.
- `internal/compose`: assertion on schema mismatch produces the expected error text (for pin-by-test, so future bumps regenerate the expected string).
- `cmd/lockfile-update`: `--commit` end-to-end test using a local bare repo fixture. Covers: no-diff exit 0, diff present → commit + push, dirty-tree refusal.

### 6.2 Smoke

`test/smoke/run.sh` asserts the sidecar for the bundle under test includes `schema_version` (non-zero integer) and `kind` matching the expected kind.

### 6.3 Compat harness

No new fixtures required for this slice — the harness exists and its surface is unchanged. The flow-level change is that harness jobs now run *after* `update-lock` in the same pipeline, against bundles the same PR just published.

### 6.4 Release isolation invariant test

New Go test `test/invariants/release_lockfile_test.go` (`+build invariants` tag, runs in `release-please.yml` and `nightly.yml`):

- For each `gh release list` entry: fetch embedded lockfile, assert every digest is resolvable on GHCR (HEAD request on manifest URL).
- Failure blocks the nightly and fails a release cut.

### 6.5 GC dry-run in CI

`gc-bundles.yml` default is dry-run. Emits a per-run artifact listing the digests it *would* prune. Maintainers review before promoting to a real-delete run via `workflow_dispatch`.

### 6.6 Coverage target

Per `CLAUDE.md`: 80% per-package. `cmd/lockfile-update`, `internal/oci`, `internal/compose`, `internal/version` additions must hold the line.

## 7. Rollout

Shipped as **two PRs** to avoid bootstrapping the new flow through the old flow:

### PR-α — Workflow flip (the new flow arrives first, empty-handed)

Contents:

- **Tα-1** — `.github/workflows/on-push.yml`: `push: true` unconditionally.
- **Tα-2** — `.github/workflows/plan-and-build.yml`: `update-lock` rewritten to commit-back to the triggering ref (not `peter-evans/create-pull-request`). Fork-PR detection + fail-fast.
- **Tα-3** — `cmd/lockfile-update`: `--commit` mode implemented with `--force-with-lease` + templated message + idempotent early-exit on no-diff.
- **Tα-4** — Docs: `CONTRIBUTING.md` "CI writes to your PR branch" note.

PR-α itself does not change any builder, catalog, or runtime code. Its own CI therefore produces no lockfile diff, and the new flow's first user-visible run happens the next time a bundle-touching PR opens.

### PR-β — Sidecar schema + enforcement (dogfoods the new flow)

Contents:

- **Tβ-1** — Sidecar extension (builder side). `builders/common/pack-bundle.sh` emits `schema_version` and `kind`. `builders/common/bundle-schema-version.env` added.
- **Tβ-2** — Planner spec-hash input. `cmd/planner` (and/or `internal/planner`) reads `bundle-schema-version.env` and folds the relevant `BUNDLE_SCHEMA_PHP_*` literal into each bundle kind's spec-hash. A schema bump therefore produces a new spec-hash → new digest → forced rebuild. Without this, the env-file bump would not re-trigger planner diff.
- **Tβ-3** — Runtime sidecar parsing (permissive). `internal/oci` parses `schema_version` + `kind`; a sidecar without `schema_version` is treated as `1` — preserves pre-slice bundles referenced by released lockfiles.
- **Tβ-4** — Runtime assertion (enforcement). `internal/compose` asserts `bundle.SchemaVersion >= internal/version.MinBundleSchema(kind)`. Hard error with expected/actual pair.
- **Tβ-5** — Core bumped to `schema_version=2` in `bundle-schema-version.env` (retrofitting the `share/php/ini/` requirement from PR #28).
- **Tβ-6** — Smoke test asserts sidecar fields present.
- **Tβ-7** — `gc-bundles.yml` replaced with the branch-aware implementation (dry-run default). `cmd/gc-bundles` added.
- **Tβ-8** — `test/invariants/release_lockfile_test.go` added; `release-please.yml` + `nightly.yml` invoke it.
- **Tβ-9** — Docs: `docs/bundle-schema-changelog.md` seeded with the core→2 bump; `docs/product-vision.md` §6.1/§6.2/§11 updated; README paragraph on "How PR CI handles bundle changes" added.

PR-β's own CI runs under the new flow (PR-α has merged). The sequence inside one CI run:

1. Planner sees `bundle-schema-version.env` change (Tβ-2 wired it into spec-hash) → core bundle's spec-hash differs from the on-branch lockfile → core rebuild queued.
2. `build-php-core.yml` rebuilds the core with Tβ-1's extended sidecar (`schema_version=2`). Pushes to GHCR.
3. `update-lock` commits the new core digest to PR-β's head ref via `cmd/lockfile-update --commit` (Tα-3).
4. Compat harness and integration tests run against the freshly committed lockfile → compose reads schema=2 → assertion passes.

If Tβ-2 is omitted, the env-file bump alone does not change the spec-hash and no rebuild is triggered — the slice silently regresses to flag-day. Tβ-2 is therefore non-optional within PR-β.

### 7.1 Release

- Conventional commits per PR (`feat(workflows): …`, `feat(cmd/lockfile-update): --commit mode`, `feat(builders/common): sidecar schema_version`, `feat(compose): bundle schema assertion`, `feat(gc): branch-aware retention`, etc.).
- `release-please` auto-cuts `v0.2.0-alpha.3` on PR-β's merge (PR-α may ship under the same tag or cut its own `alpha.2+1` depending on release-please grouping — acceptable either way).
- Post-PR-β, the core bundle digest has changed. All extension bundles rebuild implicitly because their spec-hash includes the core's digest per product-vision §6.2. The lockfile update lands in PR-β's own branch via the new flow — proving end-to-end.
- `README.md` "Compatibility / CI flow" paragraph references this spec.

### 7.1 Release

- Conventional commits (`feat(workflows): …`, `feat(builders): …`, `feat(gc): …`, `feat(compose): schema assertion`, etc.).
- `release-please` auto-cuts `v0.2.0-alpha.3` on merge.
- Bundle digest for the 8.4 core changes again (new sidecar field). Extension bundles get rebuilt too (sidecar change → spec-hash change). Lockfile update lands in the slice's own PR via the new flow — dogfoods the fix.
- `README.md` — add a one-paragraph "How PR CI handles bundle changes" section linking to this spec.

## 8. Open questions

- **Fork-PR trusted-contributor workflow.** This spec fails-fast on fork PRs. A follow-up (Phase 7) should add `safe-to-build`-labelled `pull_request_target` pipeline with hardened permissions. The contract with this spec: fork PRs are visibly blocked, never silently broken.
- **Lockfile-commit attribution.** Commits created by `cmd/lockfile-update --commit` are attributed to the `github-actions[bot]` identity. That keeps signed-commit policies intact; does any future branch-protection rule require human attribution?
- **Per-kind schema granularity.** This spec uses per-kind versions (`PHP_CORE=2`, `PHP_EXT=1`, `PHP_TOOL=1`). An alternative is a single monotonic `bundle_schema_version` with a compat table. Chosen per-kind for future independence (bumping tools schema shouldn't force a core rebuild).
- **GC performance at ~1000 open branches.** Not a Phase 2 concern, but the enumeration cost scales with branch count. If this becomes an issue, move to a cache-file-on-a-special-ref approach (a long-lived `refs/buildrush/digest-reference-index` ref that is updated by the pipeline).
- **Composite-push atomicity.** If the publish step finishes but `update-lock` fails (network, force-push race), the bundle exists on GHCR but isn't referenced. These orphans are handled by GC; no further compensation is needed.

## 9. References

- `docs/product-vision.md` §6 (distribution layer), §11 (content addressing + lockfile), §16 (versioning + reproducibility).
- `docs/superpowers/specs/2026-04-16-phased-implementation-design.md` — phase map; Phase 7 GC section.
- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` — compat-first slice (slice #1).
- `docs/superpowers/specs/2026-04-20-phase2-compat-closeout-design.md` — slice #2, the immediate trigger for this cross-cutting spec.
- `docs/superpowers/specs/2026-04-20-compat-harness-design.md` — harness workflow this spec re-wires to run post-lockfile-commit.
- PR #28, commits `b911aa5`, `fe72372`, `9c33258` — exhibit A for the failure class this spec eliminates.
- `.github/workflows/on-push.yml`, `plan-and-build.yml`, `gc-bundles.yml`, `cmd/lockfile-update/`, `builders/common/pack-bundle.sh` — the primary code surface changed.
- `CLAUDE.md` — coverage target, commit style, quality gates.
