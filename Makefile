# This works because `go list ./...` excludes vendor directories by default in modern versions of Go (1.11+).
# No need for grep or additional filtering.
TEST ?= $$(go list ./...)
TESTARGS ?=
SHELL := /bin/bash
#GOOS=darwin
#GOOS=linux
#GOARCH=amd64
VERSION=test

export CGO_ENABLED=0

readme:
	@echo "README.md generation temporarily disabled."
	@exit 0

build: build-default

version: version-default

# The following will lint only files in git. `golangci-lint run --new-from-rev=HEAD` should do it,
# but it's still including files not in git.
lint: deps lintroller gomodcheck custom-gcl
	./custom-gcl run --new-from-rev=origin/main

# Build custom golangci-lint binary with lintroller plugin.
# Uses a temporary directory to prevent git corruption during pre-commit hooks
custom-gcl: tools/lintroller/.lintroller .custom-gcl.yml
	@echo "Building custom golangci-lint binary with lintroller plugin..."
	@GOFLAGS="-buildvcs=false" golangci-lint custom
	@echo "Custom golangci-lint binary built successfully: ./custom-gcl"

# Custom linter for Atmos-specific rules (t.Setenv misuse, os.Setenv in tests, os.MkdirTemp in tests).
.PHONY: lintroller
lintroller: tools/lintroller/.lintroller
	@echo "Running lintroller (Atmos custom rules)..."
	@test -x tools/lintroller/.lintroller || (echo "Error: lintroller binary not executable" && exit 1)
	@tools/lintroller/.lintroller $(shell go list ./... | grep -v '/testdata')

tools/lintroller/.lintroller: tools/lintroller/*.go tools/lintroller/cmd/lintroller/*.go
	@echo "Building lintroller..."
	@cd tools/lintroller && go build -o .lintroller ./cmd/lintroller
	@chmod +x tools/lintroller/.lintroller
	@test -x tools/lintroller/.lintroller || (echo "Error: Failed to make lintroller executable" && exit 1)

# Check go.mod for replace/exclude directives that break go install.
.PHONY: gomodcheck
gomodcheck: tools/gomodcheck/.gomodcheck
	@tools/gomodcheck/.gomodcheck go.mod

tools/gomodcheck/.gomodcheck: tools/gomodcheck/*.go
	@echo "Building gomodcheck..."
	@cd tools/gomodcheck && go build -o .gomodcheck .
	@chmod +x tools/gomodcheck/.gomodcheck
	@test -x tools/gomodcheck/.gomodcheck || (echo "Error: Failed to make gomodcheck executable" && exit 1)

build-linux: GOOS=linux
build-linux: build-default

build-default: deps
	@echo "Building atmos $(if $(GOOS),GOOS=$(GOOS)) $(if $(GOARCH),GOARCH=$(GOARCH))"
	env $(if $(GOOS),GOOS=$(GOOS)) $(if $(GOARCH),GOARCH=$(GOARCH)) go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$(VERSION)'"

build-windows: GOOS=windows
build-windows: deps
	@echo "Building atmos for $(GOOS) ($(GOARCH))"
	go build -o build/atmos.exe -v -ldflags "-X github.com/cloudposse/atmos/pkg/version.Version=$(VERSION)"

build-macos: GOOS=darwin
build-macos: build-default

version-linux: version-default

version-macos: version-default

version-default:
	chmod +x ./build/atmos
	./build/atmos version

version-windows: build-windows
	./build/atmos.exe version

deps:
	go mod download

testacc: deps
	@echo "Running acceptance tests"
	go test $(TEST) $(TESTARGS) -timeout 40m

# Run tests with subprocess coverage collection (Go 1.20+)
testacc-cover: deps
	@scripts/collect-coverage.sh "$(TEST)" "$(TESTARGS)"

# Run acceptance tests with coverage report
testacc-coverage: testacc-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run quick tests only (skip long-running tests >2 seconds)
test-short: deps
	@echo "Running quick tests (skipping long-running tests)"
	go test -short $(TEST) $(TESTARGS) -timeout 5m

# Run quick tests with coverage
test-short-cover: deps
	@echo "Running quick tests with coverage (skipping long-running tests)"
	@GOCOVERDIR=coverage go test -short -cover $(TEST) $(TESTARGS) -timeout 5m

.PHONY: lint lintroller gomodcheck build version build-linux build-windows build-macos deps version-linux version-windows version-macos testacc testacc-cover testacc-coverage test-short test-short-cover
