# Contributing to buildrush/setup-php

## Prerequisites

- Go 1.22+
- Docker (for local bundle builds)
- Node.js 20+ (for action bootstrap linting)
- [golangci-lint](https://golangci-lint.run/welcome/install-locally/)

## Setup

```bash
git clone https://github.com/buildrush/setup-php.git
cd setup-php
go mod download
npm install
make check  # verify everything works
```

## Development Workflow

1. Create a branch from `main`
2. Make changes
3. Run `make check` — must pass before committing
4. Commit with conventional messages (`feat:`, `fix:`, `chore:`, etc.)
5. Open a PR — CI runs `ci-lint.yml` automatically

## Adding an Extension

1. Create `catalog/extensions/<name>.yaml` following the schema in existing files
2. Run `make bundle-ext EXT_NAME=<name> EXT_VERSION=<ver> PHP_ABI=8.4-nts` to test locally
3. Commit and open a PR

## Adding a PHP Version

1. Add the version to `catalog/php.yaml` under `versions:`
2. Run `make bundle-php PHP_VERSION=<ver>` to test locally
3. Commit and open a PR

## Running Tests

```bash
make test          # Go tests with race detector
make lint          # Linters
make check         # Everything (mirrors CI)
```

## Building Locally

```bash
make build                # Native platform binaries in bin/
make build-linux-amd64    # Cross-compile for Linux
```

## Project Structure

```
cmd/phpup/       — Runtime binary (what users execute)
cmd/planner/     — Build matrix planner (CI tool)
internal/        — Go packages (plan, resolve, oci, extract, compose, env, cache, catalog, lockfile, planner)
catalog/         — Declarative build specs (YAML)
builders/        — Shell scripts that compile PHP and extensions
.github/workflows/ — CI/CD pipeline
test/            — Smoke and integration tests
```
