# CLAUDE.md

## Project

`buildrush/setup-php` — A GitHub Action for fast, reproducible PHP setup using prebuilt OCI bundles.

## Specs

- Product vision: `docs/product-vision.md`
- Phase implementation design: `docs/superpowers/specs/2026-04-16-phased-implementation-design.md`
- Implementation plans: `docs/superpowers/plans/`

## Quality Gates

All code changes MUST pass `make check` before committing. This runs:
- `gofmt -s` — formatting (zero tolerance)
- `go vet` — static analysis
- `golangci-lint` — comprehensive linting (see `.golangci.yml`)
- `go mod tidy` — clean dependency state
- `go test -race -cover ./...` — tests with race detector
- `eslint` + `prettier` — Node.js code quality
- `go build ./...` — compilation check

## Architecture

- Go runtime binary (`cmd/phpup/`) resolves inputs against an embedded lockfile, fetches OCI bundles from GHCR, extracts, composes, and exports a PHP environment.
- Go planner tool (`cmd/planner/`) expands catalog YAML into GitHub Actions build matrices.
- Catalog (`catalog/`) is declarative YAML describing what to build.
- Builders (`builders/`) are shell scripts that compile PHP and extensions.
- Workflows (`.github/workflows/`) orchestrate the build pipeline.

## Code Conventions

- Follow existing patterns in the codebase.
- Go packages in `internal/` have clear single responsibilities.
- Tests use stdlib `testing` package (no testify).
- Builder scripts target bash, must work in both Docker and GitHub Actions runners.
- Workflow YAML follows the reusable workflow pattern documented in the spec.
- Lint errors MUST be fixed in code, not silenced. `//nolint` comments are only acceptable for genuine false positives and must include a justification. Existing `//nolint` comments should be removed whenever the underlying issue can be properly fixed.

## Commits

- Use conventional commit messages (feat:, fix:, chore:, docs:, test:, refactor:).
- No "Co-Authored-By" or AI attribution in commit messages or PR descriptions.
- Verify `make check` passes before every commit.

## Testing

- TDD: write failing test first, then implement.
- Every bug fix or regression fix MUST include a test that reproduces the issue before the fix and passes after.
- Target overall test coverage of 80% or higher per package. Do not let coverage regress.
- Go tests: `go test -race ./...`
- Smoke tests: `test/smoke/run.sh` for bundle verification.
- Integration tests: `.github/workflows/integration-test.yml` (CI only).
- Local bundle builds: `make bundle-php` and `make bundle-ext` (requires Docker).

## Release Engineering

- Only specific-version tags (`vX.Y.Z`) get GitHub Releases with built assets. The floating major-tag (`vN`) is a git-ref convenience for `uses: buildrush/setup-php@v1` only and MUST NOT have a corresponding GitHub Release.
- Release assets are built and uploaded inside `release-please.yml`, in the same job that creates the release. Never add a separate `push: tags:` workflow for release artifacts — `GITHUB_TOKEN`-originated tag pushes don't trigger downstream workflows, so such a workflow would silently skip release-please-created tags.
- `src/index.js:resolveReleaseTag` enforces the major-tag convention: floating-major refs (matching `/^v\d+$/`) bypass `/releases/tags/<ref>` and resolve through `/releases/latest` + major-match. Specific-version refs (matching `/^v\d+\.\d+\.\d+/`) that 404 on lookup throw instead of silently substituting `latest`, to honor pin reproducibility.
