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
	go get

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

# Directory for subprocess coverage data
COVERAGE_DIR ?= coverage

# Run tests with subprocess coverage collection (Go 1.20+)
testacc-cover: get
	@echo "Running tests with subprocess coverage collection"
	@rm -rf $(COVERAGE_DIR)
	@mkdir -p $(COVERAGE_DIR)/unit $(COVERAGE_DIR)/integration
	# Run tests with coverage enabled - subprocesses will write to GOCOVERDIR
	GOCOVERDIR=$$(pwd)/$(COVERAGE_DIR)/integration go test $(TEST) -v \
		-cover -coverpkg=./... $(TESTARGS) -timeout 40m \
		-coverprofile=$(COVERAGE_DIR)/unit.txt
	# Convert subprocess binary coverage to text format
	@if [ -d "$(COVERAGE_DIR)/integration" ] && [ "$$(ls -A $(COVERAGE_DIR)/integration 2>/dev/null)" ]; then \
		go tool covdata textfmt -i=$(COVERAGE_DIR)/integration -o=$(COVERAGE_DIR)/subprocess.txt 2>/dev/null || true; \
	fi
	# Merge unit test coverage with subprocess coverage
	@if [ -f "$(COVERAGE_DIR)/subprocess.txt" ]; then \
		go run github.com/wadey/gocovmerge@latest $(COVERAGE_DIR)/unit.txt $(COVERAGE_DIR)/subprocess.txt > coverage.raw 2>/dev/null || \
		cp $(COVERAGE_DIR)/unit.txt coverage.raw; \
	else \
		cp $(COVERAGE_DIR)/unit.txt coverage.raw; \
	fi
	# Filter out mock files
	@grep -v "mock_" coverage.raw > coverage.out || cp coverage.raw coverage.out
	@rm -f coverage.raw
	@echo "Coverage report generated: coverage.out"

# Legacy coverage mode (without subprocess coverage)
testacc-cover-legacy: get
	@echo "Running tests with coverage (legacy mode, no subprocess coverage)"
	go test $(TEST) -v -coverpkg=./... $(TESTARGS) -timeout 40m -coverprofile=coverage.out.tmp
	cat coverage.out.tmp | grep -v "mock_" > coverage.out
	@rm -f coverage.out.tmp

# Run acceptance tests with coverage report
testacc-coverage: testacc-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# View coverage percentage from binary coverage data
coverage-percent:
	@if [ -d "$(COVERAGE_DIR)" ]; then \
		go tool covdata percent -i=$(COVERAGE_DIR)/unit,$(COVERAGE_DIR)/integration 2>/dev/null || \
		go tool covdata percent -i=$(COVERAGE_DIR)/integration 2>/dev/null || \
		echo "No coverage data found"; \
	else \
		echo "No coverage directory found. Run 'make testacc-cover' first."; \
	fi

# Clean coverage data
clean-coverage:
	@rm -rf $(COVERAGE_DIR) coverage.out coverage.html coverage.*.tmp

.PHONY: lint get build version build-linux build-windows build-macos deps version-linux version-windows version-macos testacc testacc-cover testacc-cover-legacy testacc-coverage coverage-percent clean-coverage
