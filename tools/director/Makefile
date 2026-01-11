# Director - Demo scene management for Atmos VHS videos

.PHONY: build clean install test lint help

# Build configuration
BINARY_NAME := director
BUILD_DIR := .
GO := go

# Default target
all: build

## build: Build the director binary
build:
	$(GO) build -o $(BINARY_NAME) .

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)/build

## install: Install director to GOPATH/bin
install:
	$(GO) install .

## test: Run tests
test:
	$(GO) test -v ./...

## lint: Run linter
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	$(GO) fmt ./...
	gofumpt -w .

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy

## run: Build and run director (pass ARGS for arguments)
run: build
	./$(BINARY_NAME) $(ARGS)

## validate: Run director validate
validate: build
	cd ../../demos && ../tools/director/$(BINARY_NAME) validate

## render: Run director render (pass ARGS for scene names)
render: build
	cd ../../demos && ../tools/director/$(BINARY_NAME) render $(ARGS)

## install-tools: Install required tools (atmos at pinned version)
install-tools: build
	cd ../../demos && ../tools/director/$(BINARY_NAME) install

## catalog: List all demo scenes
catalog: build
	cd ../../demos && ../tools/director/$(BINARY_NAME) catalog

## help: Show this help message
help:
	@echo "Director - Demo scene management for Atmos VHS videos"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
