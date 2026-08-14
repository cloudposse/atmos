#!/usr/bin/env bash
set -e

# Directory for subprocess coverage data
COVERAGE_DIR="${COVERAGE_DIR:-coverage}"
# Final coverage profile path. Overridable so parallel CI shards (each its own
# job/process, never concurrent within one invocation) can write to distinct
# files before a later step merges/uploads them together.
COVERAGE_OUT="${COVERAGE_OUT:-coverage.out}"
TEST="${1:-./...}"
TESTARGS="${2:-}"

echo "Running tests with subprocess coverage collection"

# Clean up and create directories
rm -rf "$COVERAGE_DIR"
mkdir -p "$COVERAGE_DIR/integration"

# Run tests with coverage enabled - subprocesses will write to GOCOVERDIR.
# -covermode=atomic is pinned explicitly (rather than relying on go test's
# default) so multiple shards' profiles remain mergeable/aggregatable
# downstream instead of silently mixing covermodes.
GOCOVERDIR="$(pwd)/$COVERAGE_DIR/integration" go test $TEST \
	-cover -covermode=atomic -coverpkg=./... $TESTARGS -timeout 40m \
	-coverprofile="$COVERAGE_DIR/unit.txt"

# Convert subprocess binary coverage to text format if it exists
if [ -d "$COVERAGE_DIR/integration" ] && [ "$(ls -A "$COVERAGE_DIR/integration" 2>/dev/null)" ]; then
	go tool covdata textfmt -i="$COVERAGE_DIR/integration" -o="$COVERAGE_DIR/subprocess.txt" 2>/dev/null || true
fi

# Merge coverage files without invoking an extra Go tool. The acceptance job's
# package profile is large enough that compiling/running gocovmerge can exhaust
# constrained CI runners after tests have already passed.
{
	head -n 1 "$COVERAGE_DIR/unit.txt"
	tail -n +2 "$COVERAGE_DIR/unit.txt"
	if [ -f "$COVERAGE_DIR/subprocess.txt" ]; then
		tail -n +2 "$COVERAGE_DIR/subprocess.txt"
	fi
} > coverage.raw

# Filter out mock files - handle cross-platform grep behavior
if grep -q "mock_" coverage.raw 2>/dev/null; then
	grep -v "mock_" coverage.raw > "$COVERAGE_OUT"
else
	cp coverage.raw "$COVERAGE_OUT"
fi
rm -f coverage.raw

echo "Coverage report generated: $COVERAGE_OUT"
