.PHONY: check fmt fmt-check vet lint tidy tidy-check test test-node build clean \
        build-linux-amd64 build-linux-arm64 bundle-php bundle-ext gc-bundles-dry-run \
        ci-cell ci

# Path to the native phpup binary used by bundle-php / bundle-ext. Overridable
# so CI / power users can point at a pre-built binary.
PHPUP_BIN ?= bin/phpup

# Ensure the embedded lockfile is available for go vet/test/build
cmd/phpup/bundles.lock: bundles.lock
	@cp bundles.lock cmd/phpup/bundles.lock

# Local native build of phpup used by bundle-php / bundle-ext. Kept as a
# file target so Make only rebuilds on demand. The embedded lockfile is the
# sole declared dependency because cmd/phpup's own source files change
# rarely enough that a `make clean` + `make bin/phpup` is acceptable; if
# this becomes a friction point, widen the deps to cmd/phpup/*.go.
$(PHPUP_BIN): cmd/phpup/bundles.lock
	@mkdir -p $(dir $(PHPUP_BIN))
	go build -o $(PHPUP_BIN) ./cmd/phpup

# Pre-push check: static analysis + tests + builds. Run before every commit
# and push. For a full CI cell smoke (build php-core + exts + run fixtures
# inside bare-ubuntu docker) use `make ci-cell OS=… ARCH=… PHP=…`, which
# reproduces one cell of the `ci.yml::pipeline` matrix locally.
check: fmt-check vet lint tidy-check test test-node build
	@rm -f cmd/phpup/bundles.lock

# Format all code
fmt:
	gofmt -s -w .
	npx prettier --write src/ test/

# Check formatting without modifying (CI mode)
# Skip build/, out/, .cache/ — phpup build writes OCI layouts + extracted
# third-party source trees (e.g. swoole's Go test fixtures) under them,
# which gofmt would otherwise flag as drift.
fmt-check:
	@drift=$$(find . -name '*.go' -not -path './build/*' -not -path './out/*' -not -path './.cache/*' -not -path './.git/*' -print0 | xargs -0 gofmt -s -l); \
	  test -z "$$drift" || (echo "Files need gofmt:"; echo "$$drift"; exit 1)
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

# Build a PHP core bundle locally via phpup (docker-wrapped under the hood).
# Invocation:
#     make bundle-php PHP_VERSION=8.4 [OS=jammy] [ARCH=x86_64] [TS=nts] \
#                     [REGISTRY=oci-layout:./out/oci-layout]
# phpup docker-wraps builders/linux/build-php.sh unchanged and writes the
# resulting OCI bundle into the target registry.
bundle-php: $(PHPUP_BIN)
	$(PHPUP_BIN) build php \
		--php $(PHP_VERSION) \
		--os $(or $(OS),jammy) \
		--arch $(or $(ARCH),x86_64) \
		--ts $(or $(TS),nts) \
		--registry $(or $(REGISTRY),oci-layout:./out/oci-layout) \
		--repo .

# Build an extension bundle locally via phpup (docker-wrapped under the hood).
# Invocation:
#     make bundle-ext EXT_NAME=redis EXT_VERSION=6.2.0 PHP_ABI=8.4-nts \
#                     PHP_CORE_DIGEST=sha256:… \
#                     [OS=jammy] [ARCH=x86_64] \
#                     [REGISTRY=oci-layout:./out/oci-layout]
# Requires the prerequisite php-core already in REGISTRY (run `make bundle-php`
# first, or point REGISTRY at a remote where the core is published).
bundle-ext: $(PHPUP_BIN)
	$(PHPUP_BIN) build ext \
		--ext $(EXT_NAME) \
		--ext-version $(EXT_VERSION) \
		--php-abi $(PHP_ABI) \
		--arch $(or $(ARCH),x86_64) \
		--os $(or $(OS),jammy) \
		--php-core-digest $(PHP_CORE_DIGEST) \
		--registry $(or $(REGISTRY),oci-layout:./out/oci-layout) \
		--repo .

# Run one `ci.yml::pipeline` cell via `phpup test`: loads the published (or
# local OCI-layout) bundles for the requested OS/ARCH/PHP inside docker and
# exercises every matching row in test/compat/fixtures.yaml. Caller supplies
# OS, ARCH, PHP; REGISTRY defaults to the local OCI layout used by
# bundle-php/bundle-ext. Exercised end-to-end via `make ci`.
#
# The cell container is always linux/<ARCH>, so phpup must be cross-compiled
# to match — a darwin/arm64 host binary or a linux/amd64 host binary running
# an aarch64 cell would trip `exec format error`. We resolve the right
# bin/phpup-linux-<arch> via a shell case, rebuild both the host-native and
# linux binaries on demand, then thread the linux binary via --self-binary.
ci-cell:
	@case "$(ARCH)" in \
	  x86_64|amd64)   phpup_linux_bin="bin/phpup-linux-amd64" ;; \
	  aarch64|arm64)  phpup_linux_bin="bin/phpup-linux-arm64" ;; \
	  *) echo "ci-cell: unknown ARCH=$(ARCH), want x86_64 or aarch64" >&2; exit 2 ;; \
	esac; \
	$(MAKE) "$$phpup_linux_bin" && \
	$(MAKE) $(PHPUP_BIN) && \
	$(PHPUP_BIN) build cell \
	    --os $(OS) \
	    --arch $(ARCH) \
	    --php $(PHP) \
	    --registry $(or $(REGISTRY),oci-layout:./out/oci-layout) \
	    --repo . && \
	$(PHPUP_BIN) test \
	    --os $(OS) \
	    --arch $(ARCH) \
	    --php $(PHP) \
	    --self-binary "$$PWD/$$phpup_linux_bin" \
	    --registry $(or $(REGISTRY),oci-layout:./out/oci-layout) \
	    --fixtures test/compat/fixtures.yaml \
	    --repo .

# Cross-compiled phpup binaries used by ci-cell to mount the correct
# architecture-matching binary into the bare-ubuntu container. Declared as
# file targets so Make only rebuilds when the embedded lockfile changes.
bin/phpup-linux-amd64: cmd/phpup/bundles.lock
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/phpup-linux-amd64 ./cmd/phpup

bin/phpup-linux-arm64: cmd/phpup/bundles.lock
	@mkdir -p bin
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/phpup-linux-arm64 ./cmd/phpup

# Iterate ci-cell across jammy+noble × x86_64+aarch64 × PHP 8.1-8.5. Short-
# circuits on first failure thanks to `set -e`. Requires docker + the relevant
# bundles in REGISTRY (published GHCR by default).
ci: $(PHPUP_BIN)
	@set -e; for os in jammy noble; do \
	  for arch in x86_64 aarch64; do \
	    for php in 8.1 8.2 8.3 8.4 8.5; do \
	      echo "==> ci-cell OS=$$os ARCH=$$arch PHP=$$php"; \
	      $(MAKE) ci-cell OS=$$os ARCH=$$arch PHP=$$php; \
	    done; \
	  done; \
	done

# Clean build artifacts
clean:
	rm -rf bin/ dist/

# Dry-run the GC tool locally (requires gh auth login + network access).
gc-bundles-dry-run:
	go run ./cmd/gc-bundles --org buildrush --min-age-days 30
