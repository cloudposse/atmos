# Basic Makefile for atmos init experiment

# Build the poc binary
build: deps
	go build -o poc .

# Install dependencies
deps:
	go mod download
	go mod tidy

# Clean build artifacts
clean:
	rm -f poc

# Run tests
test: deps
	go test -v ./...

# Run tests with coverage
test-coverage: deps
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Default target
all: deps build
