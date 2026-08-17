#!/usr/bin/env bash
set -euo pipefail

# Working directory for native unit and subprocess coverage data.
COVERAGE_DIR="${COVERAGE_DIR:-coverage}"
# Native merged coverage output directory.
COVERAGE_DATA_OUT="${COVERAGE_DATA_OUT:-$COVERAGE_DIR/merged}"
# Final text coverage profile path.
COVERAGE_OUT="${COVERAGE_OUT-coverage.out}"
TEST="${1:-./...}"
TESTARGS="${2:-}"

echo "Running tests with subprocess coverage collection"

# TEST and TESTARGS are command-line fragments supplied by Atmos and may each
# contain multiple whitespace-separated arguments.
read -r -a test_packages <<< "${TEST//$'\n'/ }"
read -r -a test_args <<< "$TESTARGS"

# Clean up and create directories.
rm -rf "$COVERAGE_DIR"
mkdir -p "$COVERAGE_DIR/integration" "$COVERAGE_DIR/unit"
coverage_dir_abs=$(cd "$COVERAGE_DIR" && pwd)

# Run tests with coverage enabled. The test binaries write native data to the
# unit directory, while covered subprocesses inherit GOCOVERDIR and write to
# the integration directory.
# -covermode=atomic is pinned explicitly (rather than relying on go test's
# default) so multiple shards' profiles remain mergeable/aggregatable
# downstream instead of silently mixing covermodes.
GOCOVERDIR="$coverage_dir_abs/integration" go test "${test_packages[@]}" \
	-cover -covermode=atomic -coverpkg=./... "${test_args[@]}" -timeout "${GO_TEST_TIMEOUT:-40m}" \
	-args -test.gocoverdir="$coverage_dir_abs/unit"

coverage_inputs=("$COVERAGE_DIR/unit")
if find "$COVERAGE_DIR/integration" -maxdepth 1 -type f -name 'covmeta.*' -print -quit | grep -q .; then
	coverage_inputs+=("$COVERAGE_DIR/integration")
fi

COVERAGE_DATA_OUT="$COVERAGE_DATA_OUT" COVERAGE_OUT="$COVERAGE_OUT" \
	scripts/merge-coverage.sh "${coverage_inputs[@]}"
