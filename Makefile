.DEFAULT_GOAL := help

BINARY   := gitpersona
PKG      := github.com/weikxs/gitpersona
BIN_DIR  := bin
COVERAGE := coverage.out
MIN_COVERAGE := 95
# Tests keep all their files under /tmp, never in the real home directory.
TEST_TMPDIR := /tmp/gitpersona-test
# internal/app runs real git hundreds of times: about 8 minutes with -race on a fast Mac, more on CI runners,
# so the timeout is above Go's default of 10 minutes.
GO_TEST := mkdir -p $(TEST_TMPDIR) && TMPDIR=$(TEST_TMPDIR) go test -race -timeout 20m

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.version=$(VERSION) \
	-X $(PKG)/internal/version.commit=$(COMMIT) \
	-X $(PKG)/internal/version.date=$(DATE)

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/$(BINARY)

.PHONY: test
test: ## Run all tests with the race detector (temp files under /tmp)
	$(GO_TEST) ./...

.PHONY: cover
cover: ## Run tests and fail below the minimum coverage
	$(GO_TEST) -coverprofile=$(COVERAGE) ./...
	@go tool cover -func=$(COVERAGE) | awk -v min=$(MIN_COVERAGE) '/^total:/ { sub("%", "", $$3); printf "total coverage: %s%% (minimum %s%%)\n", $$3, min; if ($$3 + 0 < min) exit 1 }'

.PHONY: e2e
e2e: ## Drive the configuration page in headless Chrome (needs Chrome; GITPERSONA_CHROME picks one)
	mkdir -p $(TEST_TMPDIR) && GITPERSONA_E2E=1 TMPDIR=$(TEST_TMPDIR) go test -race -count=1 ./test/e2e/

.PHONY: fmt
fmt: ## Format the code
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file is not gofmt-formatted
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "not gofmt-formatted:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: screenshots
screenshots: ## Draw the README pictures in docs/images from a demo home (needs Chrome)
	@mkdir -p docs/images
	GITPERSONA_E2E=1 GITPERSONA_SHOTS=$(CURDIR)/docs/images go test ./test/e2e -run TestREADMEScreenshots -count=1

.PHONY: lint
lint: ## Run golangci-lint (install: brew install golangci-lint)
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found; install it with: brew install golangci-lint"; exit 1; }
	golangci-lint run

.PHONY: privacy
privacy: ## Block real identities, local paths and secrets from being committed
	./scripts/check-privacy.sh

.PHONY: check
check: fmt-check vet privacy cover ## Everything CI runs except lint
	@echo "all checks passed"

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

.PHONY: hooks
hooks: ## Install the repository git hooks
	git config core.hooksPath .githooks
	@echo "git hooks installed from .githooks/"

.PHONY: snapshot
snapshot: ## Build release archives locally without publishing (install: brew install goreleaser)
	@command -v goreleaser >/dev/null || { echo "goreleaser not found; install it with: brew install goreleaser"; exit 1; }
	goreleaser release --snapshot --clean

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist $(COVERAGE)
