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

get:
	@go get

build: build-default

version: version-default

# The following will lint only files in git. `golangci-lint run --new-from-rev=HEAD` should do it,
# but it's still including files not in git.
lint: get
	golangci-lint run --new-from-rev=origin/main

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

testacc: get
	@echo "Running acceptance tests"
	go test $(TEST) -v $(TESTARGS) -timeout 40m

testacc-cover: get
	@echo "Running tests with coverage"
	go test $(TEST) -v -coverpkg=./... $(TESTARGS) -timeout 40m -coverprofile=coverage.out

# Run acceptance tests with coverage report
testacc-coverage: testacc-cover
	go tool cover -html=coverage.out -o coverage.html

# The actual gotcha binary path
GOTCHA_BIN := $(shell go env GOPATH)/bin/gotcha

# Build and install gotcha tool - depends on actual binary file
$(GOTCHA_BIN):
	@$(MAKE) -C tools/gotcha install

# Test target for CI with gotcha
testacc-ci: get $(GOTCHA_BIN)
	$(GOTCHA_BIN) stream ./... \
		--show=all \
		--timeout=40m \
		--coverprofile=coverage.out \
		--output=test-results.json \
		-- -coverpkg=github.com/cloudposse/atmos/... $(TESTARGS)
	$(GOTCHA_BIN) parse test-results.json --format=github --coverprofile=coverage.out --post-comment

# Clean test artifacts
clean-test:
	rm -f test-results.json test-summary.md coverage.out coverage.html

.PHONY: lint get build version build-linux build-windows build-macos deps version-linux version-windows version-macos testacc testacc-cover testacc-coverage testacc-ci clean-test
