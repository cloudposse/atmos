#!/usr/bin/env bash
set -e

# Directory for subprocess coverage data
COVERAGE_DIR="${COVERAGE_DIR:-coverage}"
TEST="${1:-./...}"
TESTARGS="${2:-}"

echo "Running tests with subprocess coverage collection"

# Clean up and create directories
rm -rf "$COVERAGE_DIR"
mkdir -p "$COVERAGE_DIR/integration"

# Run tests with coverage enabled - subprocesses will write to GOCOVERDIR
GOCOVERDIR="$(pwd)/$COVERAGE_DIR/integration" go test $TEST \
    -cover -coverpkg=./... $TESTARGS -timeout 40m \
    -coverprofile="$COVERAGE_DIR/unit.txt"

# Convert subprocess binary coverage to text format if it exists
if [ -d "$COVERAGE_DIR/integration" ] && [ "$(ls -A "$COVERAGE_DIR/integration" 2>/dev/null)" ]; then
    go tool covdata textfmt -i="$COVERAGE_DIR/integration" -o="$COVERAGE_DIR/subprocess.txt" 2>/dev/null || true
fi

# Merge coverage files if subprocess coverage exists
if [ -f "$COVERAGE_DIR/subprocess.txt" ]; then
    # Try to use gocovmerge if available, otherwise just use unit coverage
    go run github.com/wadey/gocovmerge@latest \
        "$COVERAGE_DIR/unit.txt" \
        "$COVERAGE_DIR/subprocess.txt" > coverage.raw 2>/dev/null || \
    cp "$COVERAGE_DIR/unit.txt" coverage.raw
else
    cp "$COVERAGE_DIR/unit.txt" coverage.raw
fi

# Filter out mock files - handle cross-platform grep behavior
if grep -q "mock_" coverage.raw 2>/dev/null; then
    grep -v "mock_" coverage.raw > coverage.out
else
    cp coverage.raw coverage.out
fi
rm -f coverage.raw

echo "Coverage report generated: coverage.out"
