#!/usr/bin/env bash
set -euo pipefail

# Native coverage output directory. The directory is replaced on each run.
COVERAGE_DATA_OUT="${COVERAGE_DATA_OUT:-coverage/merged}"
# Text coverage profile. Set to an empty string when only native data is needed.
COVERAGE_OUT="${COVERAGE_OUT-coverage.out}"

if [ "$#" -eq 0 ]; then
	echo "usage: $0 <coverage-data-dir> [<coverage-data-dir> ...]" >&2
	exit 1
fi

case "$COVERAGE_DATA_OUT" in
	"" | "/" | ".")
		echo "refusing unsafe COVERAGE_DATA_OUT: '$COVERAGE_DATA_OUT'" >&2
		exit 1
		;;
esac

for coverage_input in "$@"; do
	if ! find "$coverage_input" -maxdepth 1 -type f -name 'covmeta.*' -print -quit | grep -q .; then
		echo "no native coverage metadata found in $coverage_input" >&2
		exit 1
	fi
done

coverage_inputs=$(IFS=,; printf '%s' "$*")
rm -rf -- "$COVERAGE_DATA_OUT"
mkdir -p "$COVERAGE_DATA_OUT"

# -pcombine unions counters from separately built test and application binaries.
go tool covdata merge -pcombine -i="$coverage_inputs" -o="$COVERAGE_DATA_OUT"

if [ -n "$COVERAGE_OUT" ]; then
	coverage_raw=$(mktemp)
	trap 'rm -f "$coverage_raw"' EXIT

	go tool covdata textfmt -i="$COVERAGE_DATA_OUT" -o="$coverage_raw"
	mkdir -p "$(dirname "$COVERAGE_OUT")"
	if grep -q "mock_" "$coverage_raw" 2>/dev/null; then
		grep -v "mock_" "$coverage_raw" > "$COVERAGE_OUT"
	else
		cp "$coverage_raw" "$COVERAGE_OUT"
	fi

	echo "Coverage report generated: $COVERAGE_OUT"
fi

echo "Native coverage data generated: $COVERAGE_DATA_OUT"
