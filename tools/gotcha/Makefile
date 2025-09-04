# Disable CGO for static binaries
export CGO_ENABLED=0

# Variables
BINARY_NAME := gotcha
BUILD_DIR := build
GOPATH := $(shell go env GOPATH)
INSTALL_PATH := $(GOPATH)/bin

# Default target
build: build-default

# Download module dependencies
mod-download:
	@go mod download

# Build the gotcha binary
build-default: mod-download
	@echo "Building gotcha to $(BUILD_DIR)/$(BINARY_NAME)"
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) -v ./

# Install to GOPATH/bin
install: build-default
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)"
	@mkdir -p $(INSTALL_PATH)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_PATH)/$(BINARY_NAME)"

# Uninstall from GOPATH/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(INSTALL_PATH)"
	@rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Removed $(INSTALL_PATH)/$(BINARY_NAME)"

# Run tests
test: mod-download
	@echo "Running tests"
	go test ./...

# Lint only changed files from origin/main
lint: mod-download
	golangci-lint run --new-from-rev=origin/main

# Format code with gofumpt
fmt:
	gofumpt -w .

# Clean build artifacts and generated files
clean: uninstall
	@echo "Cleaning build artifacts and generated files"
	rm -rf $(BUILD_DIR)/
	rm -f test-results.json
	rm -f test-summary.md
	rm -f *.out coverage.html

.PHONY: build build-default mod-download install uninstall test lint fmt clean