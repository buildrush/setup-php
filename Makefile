.PHONY: check fmt fmt-check vet lint tidy tidy-check test test-node build clean \
        build-linux-amd64 build-linux-arm64 bundle-php bundle-ext

# Ensure the embedded lockfile is available for go vet/test/build
cmd/phpup/bundles.lock: bundles.lock
	@cp bundles.lock cmd/phpup/bundles.lock

# Run all checks locally (mirrors CI exactly)
check: fmt-check vet lint tidy-check test test-node build
	@rm -f cmd/phpup/bundles.lock

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
		ubuntu:24.04 \
		bash -c "apt-get update && apt-get install -y curl xz-utils && ./builders/linux/build-php.sh"

# Build an extension bundle locally via Docker
bundle-ext:
	docker run --rm \
		-v $$(pwd):/workspace -w /workspace \
		-e EXT_NAME=$(EXT_NAME) \
		-e EXT_VERSION=$(EXT_VERSION) \
		-e PHP_ABI=$(PHP_ABI) \
		ubuntu:24.04 \
		bash -c "apt-get update && apt-get install -y curl && ./builders/linux/build-ext.sh"

# Clean build artifacts
clean:
	rm -rf bin/ dist/
