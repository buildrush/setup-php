# Compat Harness — CI Verification vs. `shivammathur/setup-php@v2`

**Status:** Design, awaiting implementation plan
**Date:** 2026-04-20
**Supersedes:** Nothing (follow-up slice #1 from `2026-04-17-phase2-compat-slice-design.md` §8)
**Target release:** Ships under the Phase 2 umbrella; no release bump of its own.

## 1. Summary

This slice adds a CI workflow that, on every PR to `main` and every push to `main`, runs a fixture matrix through both `buildrush/setup-php` (the code under review) and `shivammathur/setup-php@v2` (pinned by SHA), captures a normalized probe of the resulting PHP environment from each, and fails the check on any difference not listed in `docs/compat-matrix.md`'s allowlist.

It answers two questions per matrix cell:

1. **Does our action work?** The probe from the `ours` job has to succeed and emit plausible output.
2. **Do we match `shivammathur/setup-php@v2`?** The probe from `ours` has to equal the probe from `theirs` after path normalization and allowlist application.

Splitting functional correctness from compat equivalence keeps failure attribution clean: a red cell tells you immediately whether *we* broke or *we diverged from v2*.

## 2. Goals & non-goals

### Goals
- Run on `pull_request: [main]`, `push: [main]`, and `workflow_dispatch`.
- Exercise the version/platform/arch axes that are **actually built today** (PHP 8.4, `ubuntu-24.04`, `x86_64`), plus the 6 fixture flavors enumerated in §5.3.
- Detect regressions and intentional compat drift the same way: any unexplained diff fails; intentional drift is added to the allowlist in the same PR that introduces it.
- Pin `shivammathur/setup-php` by commit SHA; piggy-back on the pin already captured in `docs/compat-matrix.md`.
- Grow naturally with Phase 2: the fixtures manifest is the single file to extend as later slices add versions, arches, distros, or extensions.

### Non-goals
- Multi-version (8.1, 8.2, 8.3, 8.5). Those don't build yet — phase-2 follow-up slices.
- Multi-arch (`aarch64`). Follow-up slice.
- Multi-distro (`ubuntu-22.04`). Follow-up slice.
- `macOS` / `Windows`. Phases 5/6.
- Nightly schedule. Out of scope; if drift outside CI becomes a concern, a nightly trigger is a one-line workflow addition.
- Tool installation (`tools:` input). Already a no-op + warn in Phase 2 compat slice; parity for it lands when Phase 3 adds real tool support.
- A local-runnable "compat in Go" variant. The harness is CI-only for this slice.

## 3. Source of truth

Compat equivalence is measured against the pin already recorded in `docs/compat-matrix.md`:

| Field | Value |
| --- | --- |
| `shivammathur/setup-php` SHA | `accd6127cb78bee3e8082180cb391013d204ef9f` |
| Ondrej PPA snapshot | `2026-04-11` |

The harness workflow references `shivammathur/setup-php@<SHA>` directly, not the floating `@v2` tag, so a Dependabot-style PR that bumps the SHA re-runs the diff and surfaces any drift for deliberate review.

## 4. Scope matrix

This slice covers only what phase 2 slice #1 actually builds.

| Axis | In scope this slice | Source |
|------|---------------------|--------|
| PHP version | `8.4` only | `catalog/php.yaml` has `sources:` only on `8.4` |
| OS | `ubuntu-24.04` | Existing runners; Phase 2 scope |
| Arch | `x86_64` | Existing build; Phase 2 scope |
| Fixtures | 6 hand-authored, see §5.3 | This doc |
| Triggers | PR to main, push to main, `workflow_dispatch` | §5.1 |

The fixture axes (and only the fixture axes) are declared in `test/compat/fixtures.yaml`; later Phase 2 slices extend that file when they extend the axes above.

## 5. Architecture

### 5.1 Workflow layout

New file `.github/workflows/compat-harness.yml`:

```
compat-harness.yml
├── build              (1 job)                   builds phpup, uploads artifact — same pattern as integration-test.yml
├── fixtures           (1 job)                   reads test/compat/fixtures.yaml, emits matrix include list as job output
├── ours/<fixture>     (N jobs, one per fixture) runs ./  + test/compat/probe.sh → uploads ours.json
├── theirs/<fixture>   (N jobs, one per fixture) runs shivammathur/setup-php@<SHA> + test/compat/probe.sh → uploads theirs.json
├── diff/<fixture>     (N jobs, one per fixture) downloads both JSONs, runs cmd/compat-diff/
└── compat-gate        (1 job)                   re-actors/alls-green over everything — mirrors integration-test.yml's gate
```

Matrix over fixtures uses `strategy.matrix` with `fail-fast: false` so one bad cell doesn't mask the others.

Triggers:

```yaml
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
  workflow_dispatch:
```

Permissions: `contents: read`, `packages: read` (same as `integration-test.yml`).

### 5.2 The probe

`test/compat/probe.sh` is a bash script that takes one argument — the output JSON path — and writes a normalized snapshot after `setup-php` (whichever implementation) has run. It must be identical across `ours` and `theirs` jobs; it only cares about the resulting PHP environment, not which action produced it.

Probe contents (all fields present and deterministic):

```json
{
  "php_version": "8.4.5",
  "sapi": "cli",
  "zts": false,
  "extensions": ["Core", "ctype", "curl", "..."],
  "ini": {
    "memory_limit": "128M",
    "date.timezone": "UTC",
    "error_reporting": "E_ALL",
    "disable_functions": "",
    "opcache.enable": "1",
    "opcache.enable_cli": "1",
    "...": "..."
  },
  "env_delta": ["PHP_INI_SCAN_DIR", "..."],
  "path_additions": ["<PHP_ROOT>/bin"]
}
```

Normalization rules implemented in the probe:
- `php_version` uses the full `major.minor.patch` from `PHP_VERSION` constant — not the short form.
- `extensions` is the sorted output of `php -m`, lowercased, with empty lines and the `[Zend Modules]` / `[PHP Modules]` headers stripped.
- `ini` is the result of `php -r` calling `ini_get()` on a curated key list stored in `test/compat/ini-keys.txt`. The fixture can extend that list by writing a `ini-keys.extra` file into the fixture workspace before the probe runs.
- `env_delta` is the list of env var **names** (not values) that exist after `setup-php` ran but did not exist before. Values are omitted because they contain absolute paths.
- `path_additions` is the list of directory entries added to `$PATH` by `setup-php`. Each entry is normalized by matching it against a fixed set of patterns (`.*/PHP/\d+\.\d+\.\d+/.*` → `<PHP_ROOT>/<suffix>`, and so on). The pattern table lives in the probe script; it fails loudly if an unrecognized path shape appears.

No timestamps, no PIDs, no file mtimes. Re-running the probe on the same install must produce a byte-identical JSON.

### 5.3 Fixtures

Six fixtures, hand-authored in `test/compat/fixtures.yaml` keyed by name:

```yaml
# test/compat/fixtures.yaml
fixtures:
  - name: bare
    php-version: "8.4"
    extensions: ""
    ini-values: ""
    coverage: "none"
  - name: single-ext
    php-version: "8.4"
    extensions: "redis"
    ini-values: ""
    coverage: "none"
  - name: multi-ext
    php-version: "8.4"
    extensions: "redis, xdebug"
    ini-values: ""
    coverage: "none"
  - name: exclusion
    php-version: "8.4"
    extensions: ":opcache"
    ini-values: ""
    coverage: "none"
  - name: none-reset
    php-version: "8.4"
    extensions: "none, redis"
    ini-values: ""
    coverage: "none"
  - name: ini-and-coverage
    php-version: "8.4"
    extensions: ""
    ini-values: "memory_limit=256M,date.timezone=UTC"
    coverage: "xdebug"
```

Rationale — each fixture touches one compat layer (see the layer table in the phase-2 compat slice design):

| Fixture | Layer exercised |
|---------|-----------------|
| `bare` | L1/L3 — default ini + default built-in set |
| `single-ext` | L2 — add one PECL extension |
| `multi-ext` | L2 — two PECL extensions, ordering |
| `exclusion` | L2 — `:ext` exclusion of a built-in |
| `none-reset` | L2 — `none` reset semantics |
| `ini-and-coverage` | L2/L3 — user ini overrides + coverage driver |

The fixtures YAML is consumed by the workflow via a small step that reads it and sets up the `strategy.matrix.include` list (GitHub Actions expression or a generator step — see §5.6).

### 5.4 The diff tool

New Go binary `cmd/compat-diff/`. Single-purpose: compare two probe JSONs with allowlist support.

Invocation:

```
compat-diff \
  --ours  ours.json \
  --theirs theirs.json \
  --allowlist docs/compat-matrix.md \
  --fixture <fixture-name>
```

Exit codes:
- `0` — match (after normalization + allowlist application)
- `1` — diff not covered by allowlist (prints annotated path + old/new values, GitHub Actions `::error::` format)
- `2` — malformed input (allowlist doc, JSON, unknown fixture name)

Allowlist loading: the tool extracts a fenced YAML block from `docs/compat-matrix.md` between a pair of canonical markers (`<!-- compat-harness:deviations:start -->` / `<!-- compat-harness:deviations:end -->`). The block's shape:

```yaml
deviations:
  - path: ini.opcache.jit_buffer_size
    kind: ignore      # "ignore" = drop from both sides before comparing
    reason: "v2 sets 256M on Linux; we set 0 pending JIT support (follow-up: #123)"
    fixtures: ["*"]   # glob list; "*" means all fixtures
  - path: extensions
    kind: allow       # "allow" = both values may differ but both must be non-empty
    reason: "..."
    fixtures: ["none-reset"]
```

The two `kind`s cover the realistic cases: `ignore` (drop the field before diff; used when v2 does something we explicitly don't plan to match), and `allow` (permit any value as long as both sides produce *some* value; used sparingly). Anything richer is not needed for this slice — complicated conditional logic in the allowlist becomes its own compat bug.

### 5.5 Data flow per fixture

```
ours/<fixture>                                theirs/<fixture>
──────────────                                ────────────────
checkout                                      checkout
download phpup artifact                       (no artifact needed)
place phpup in RUNNER_TOOL_CACHE              uses: shivammathur/setup-php@<SHA>
uses: ./                                        with fixture inputs
  with fixture inputs                         bash test/compat/probe.sh theirs.json
bash test/compat/probe.sh ours.json           upload-artifact theirs-<fixture>
upload-artifact ours-<fixture>

                     └──────┬──────┘
                            ▼
                diff/<fixture>
                ──────────────
                checkout
                download-artifact ours-<fixture>
                download-artifact theirs-<fixture>
                go run ./cmd/compat-diff \
                    --ours ours.json --theirs theirs.json \
                    --allowlist docs/compat-matrix.md \
                    --fixture <fixture>
                → exit 0 (pass) or non-zero (fail, PR blocked)
```

### 5.6 Matrix generation

The workflow reads `test/compat/fixtures.yaml` once in a tiny `fixtures` job that emits a matrix include list as a job output:

```yaml
fixtures:
  runs-on: ubuntu-24.04
  outputs:
    matrix: ${{ steps.load.outputs.matrix }}
  steps:
    - uses: actions/checkout@v6
    - id: load
      run: |
        matrix=$(yq -o=json '{include: .fixtures}' test/compat/fixtures.yaml)
        echo "matrix=$matrix" >> "$GITHUB_OUTPUT"
```

Downstream jobs (`ours`, `theirs`, `diff`) reference `needs.fixtures.outputs.matrix` as their `strategy.matrix`. This keeps fixture additions a data-only change.

### 5.7 Placement relative to existing workflows

| Workflow | Answers | Keeps |
|---|---|---|
| `integration-test.yml` (existing) | *Does our action install and run?* | Its current 4-fixture smoke on PHP 8.4. Untouched. |
| `compat-harness.yml` (new, this slice) | *Do we match v2 byte-for-byte modulo allowlist?* | Own triggers, own matrix, own gate. |

The two have different failure modes and will evolve at different cadences. Keeping them separate means one flaky cell doesn't poison the other's required-check state.

## 6. Error handling & failure policy

- **`diff` job non-zero** → cell fails → `compat-gate` fails → PR blocked.
- **Unblock path 1:** fix the code (update `internal/compat`, builder flags, default ini, etc.) so the diff goes away.
- **Unblock path 2:** add a deviation entry to `docs/compat-matrix.md` inside the same PR. Each entry carries a `reason` and (optionally) an `issue` link.
- **Allowlist additions are reviewable** because the deviations block is part of the authoritative compat doc — the reviewer sees both the code change and the compat-contract change in the same diff.
- **Probe script failure** (unrecognized path shape, missing PHP binary, malformed `php -m`) → cell fails with `::error::` annotation; no special-casing.
- **`compat-diff` malformed-input exit (2)** is distinguished from diff-found (1) in the annotation so reviewers can triage.

## 7. Testing

Every piece the harness depends on gets its own tests; the harness itself is exercised end-to-end.

- **`test/compat/probe_test.go`** — a Go test that runs `probe.sh` against a synthetic PHP environment (stubbed `php` binary that prints fixed output, fixed env vars) and asserts the emitted JSON matches a golden file. Covers: normalization of `$PATH` additions, env var name extraction, extension sort order, curated-ini-key lookup, presence of extra-ini-keys file.
- **`cmd/compat-diff/*_test.go`** — table-driven tests covering:
  - exact JSON match → exit 0
  - single key difference, no allowlist → exit 1, error mentions path
  - single key difference, allowlist `ignore` match → exit 0
  - single key difference, allowlist `allow` + both sides non-empty → exit 0
  - single key difference, allowlist `allow` + one side empty → exit 1
  - allowlist markers missing from doc → exit 2
  - malformed YAML inside markers → exit 2
  - unknown fixture name passed on CLI → exit 2
  - fixture glob filtering (`fixtures: ["single-ext"]` with current=`multi-ext`) skips the entry
- **No new unit coverage inside `internal/compat`** — that package's tests stay as defined in the phase-2 compat slice. The harness is a black-box consumer.
- **Coverage target** per `CLAUDE.md` (80% per package) applies to the new `cmd/compat-diff` and any supporting `internal/` package the diff tool adds.
- **Smoke of the harness itself** before merge: running the new workflow on a draft PR against a branch that deliberately bumps one ini default; the expected outcome is one red cell pointing at that key, matched by an allowlist entry in the same PR.

## 8. Release

- Conventional commits (`feat(workflows): add compat harness`, `feat: add cmd/compat-diff`, etc.).
- No user-visible behavior change → no release-please bump of its own; lands whenever it's merged and ships with the next Phase 2 alpha.
- `README.md` gets a one-paragraph "How we verify compat with v2" note linking to the harness workflow and `docs/compat-matrix.md`.

## 9. Open questions

- **`yq` on the runner.** `ubuntu-24.04` GitHub-hosted runners have `yq` preinstalled; no extra setup step needed. If a future runner image drops it, the `fixtures` job grows one `pip install yq` line. Not worth designing around today.
- **Allowlist block format.** YAML fenced inside Markdown comment markers is the simplest reviewable option that keeps the allowlist discoverable in `compat-matrix.md`. Open to a separate file (`docs/compat-allowlist.yaml`) if a reviewer of this spec prefers — trade-off is one more place to keep in sync versus colocation with the prose.
- **`env_delta` values.** The spec deliberately omits values. If a later investigation shows that value drift (e.g. `PHP_INI_SCAN_DIR` pointing at the wrong conf.d) is a common bug source, the probe grows a normalized-value field; until then, names-only keeps the JSON byte-stable.

## 10. Follow-ups (out of this slice)

- Extend `test/compat/fixtures.yaml` as later Phase 2 slices expand axes (new PHP versions, `aarch64`, `ubuntu-22.04`, top-50 extensions). No workflow changes needed — the matrix is generated.
- Nightly trigger if PR/main coverage proves insufficient.
- macOS/Windows once Phases 5/6 land.
- `tools:` input parity once Phase 3 adds real tool support.

## 11. References

- `docs/superpowers/specs/2026-04-17-phase2-compat-slice-design.md` §8 — names this slice as the first Phase 2 follow-up.
- `docs/compat-matrix.md` — pinned SHA/PPA metadata; will host the deviations allowlist block.
- `.github/workflows/integration-test.yml` — pattern for `build` job + `alls-green` gate reused here.
- `CLAUDE.md` — quality gates (must pass `make check`), commit style, coverage target.
