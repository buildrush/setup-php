.PHONY: check check-fast fmt fmt-check vet lint tidy tidy-check test test-node build clean \
        build-linux-amd64 build-linux-arm64 bundle-php bundle-ext gc-bundles-dry-run \
        local-ci

# Ensure the embedded lockfile is available for go vet/test/build
cmd/phpup/bundles.lock: bundles.lock
	@cp bundles.lock cmd/phpup/bundles.lock

# Full pre-push check: static analysis + tests + builds + a docker smoke that
# exercises the published bundles on both jammy and noble runners. Takes ~5
# minutes when the bundle caches are cold. Use check-fast for rapid iteration
# during development; use check before every push.
check: fmt-check vet lint tidy-check test test-node build local-ci
	@rm -f cmd/phpup/bundles.lock

# Fast check: skip the docker-based local-ci. Keep PR-authors productive during
# rapid iteration; real gate is the full `check` target before push.
check-fast: fmt-check vet lint tidy-check test test-node build
	@rm -f cmd/phpup/bundles.lock

# Docker-based smoke that mirrors the compat-harness: pulls published bundles,
# composes them into a mock /opt/buildrush tree, and loads PHP + a multi-ext
# fixture in both ubuntu:22.04 and ubuntu:24.04 containers. Catches cross-OS
# runtime-dep / rpath regressions locally in ~3-5 min instead of a 30-min CI
# cycle. Skipped gracefully if docker is unavailable.
local-ci:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "local-ci: docker not found, skipping (fix: install Docker to run this check)"; \
		exit 0; \
	fi
	@./test/smoke/local-ci.sh

# Format all code
fmt:
	gofmt -s -w .
	npx prettier --write src/ test/

# Check formatting without modifying (CI mode)
fmt-check:
	@test -z "$$(gofmt -s -l .)" || (echo "Files need gofmt:"; gofmt -s -l .; exit 1)
	npx prettier --check src/ test/

# Go vet
vet: cmd/phpup/bundles.lock
	go vet ./...

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...
	npx eslint src/ test/

# Go mod tidy
tidy:
	go mod tidy

# Verify go.sum is clean
tidy-check:
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak 2>/dev/null || true
	@go mod tidy
	@diff go.mod go.mod.bak > /dev/null && diff go.sum go.sum.bak > /dev/null 2>&1 || \
		(echo "go.mod/go.sum is dirty after tidy"; mv go.mod.bak go.mod; mv go.sum.bak go.sum 2>/dev/null; exit 1)
	@rm -f go.mod.bak go.sum.bak

# Test with race detector
test: cmd/phpup/bundles.lock
	go test -race -cover ./...

# Node unit tests
test-node:
	npm test

# Build all binaries (native platform)
build: cmd/phpup/bundles.lock
	go build -o bin/phpup ./cmd/phpup
	go build -o bin/planner ./cmd/planner

# Cross-compile for Linux
build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/phpup-linux-amd64 ./cmd/phpup
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/planner-linux-amd64 ./cmd/planner

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/phpup-linux-arm64 ./cmd/phpup
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/planner-linux-arm64 ./cmd/planner

# Build a PHP core bundle locally via Docker
bundle-php:
	docker run --rm \
		-v $$(pwd):/workspace -w /workspace \
		-e PHP_VERSION=$(PHP_VERSION) \
		-e ARCH=$(or $(ARCH),x86_64) \
		ubuntu:22.04 \
		bash -c "apt-get update && apt-get install -y curl xz-utils && ./builders/linux/build-php.sh"

# Build an extension bundle locally via Docker
bundle-ext:
	docker run --rm \
		-v $$(pwd):/workspace -w /workspace \
		-e EXT_NAME=$(EXT_NAME) \
		-e EXT_VERSION=$(EXT_VERSION) \
		-e PHP_ABI=$(PHP_ABI) \
		ubuntu:22.04 \
		bash -c "apt-get update && apt-get install -y curl && ./builders/linux/build-ext.sh"

# Clean build artifacts
clean:
	rm -rf bin/ dist/

# Dry-run the GC tool locally (requires gh auth login + network access).
gc-bundles-dry-run:
	go run ./cmd/gc-bundles --org buildrush --min-age-days 30
