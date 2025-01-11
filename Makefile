# This works because `go list ./...` excludes vendor directories by default in modern versions of Go (1.11+).
# No need for grep or additional filtering.
TEST ?= $$(go list ./...)
SHELL := /bin/bash
#GOOS=darwin
#GOOS=linux
#GOARCH=amd64
VERSION=test

# List of targets the `readme` target should call before generating the readme
export README_DEPS ?= docs/targets.md

-include $(shell curl -sSL -o .build-harness "https://cloudposse.tools/build-harness"; echo .build-harness)

## Lint terraform code
lint:
	$(SELF) terraform/install terraform/get-modules terraform/get-plugins terraform/lint terraform/validate

get:
	go get

build: build-default

version: version-default

build-linux: GOOS=linux
build-linux: build-default

build-default: get
	@echo "Building atmos $(if $(GOOS),GOOS=$(GOOS)) $(if $(GOARCH),GOARCH=$(GOARCH))"
	env $(if $(GOOS),GOOS=$(GOOS)) $(if $(GOARCH),GOARCH=$(GOARCH)) go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$(VERSION)'"

build-windows: GOOS=windows
build-windows: get
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

# Test verbosity levels: quiet, normal, verbose
TEST_VERBOSITY ?= normal

.PHONY: testacc
testacc: ## Run all tests
	@if [ "$(TEST_VERBOSITY)" = "quiet" ]; then \
		ATMOS_TEST_VERBOSITY=$(TEST_VERBOSITY) go test ./... -timeout 20m 2>&1 | grep -E "^(ok|FAIL|panic:|--- FAIL)" | grep -v "=== RUN"; \
	else \
		ATMOS_TEST_VERBOSITY=$(TEST_VERBOSITY) go test -v ./... -timeout 20m | grep -E "^(ok|FAIL|panic:|--- FAIL|=== RUN)"; \
	fi

.PHONY: lint get build version build-linux build-windows build-macos deps version-linux version-windows version-macos testacc
