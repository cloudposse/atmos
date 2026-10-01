#!/usr/bin/env bash
set -euo pipefail

readonly minimum_coverage=90
profile="$(mktemp -t atmos-function-parser-coverage.XXXXXX)"
trap 'rm -f "$profile"' EXIT

go test ./pkg/function/parser/... -covermode=atomic -coverprofile="$profile"
coverage="$(go tool cover -func="$profile" | awk '/^total:/ { gsub("%", "", $3); print $3 }')"

if [[ -z "$coverage" ]]; then
  echo "Unable to determine function parser coverage" >&2
  exit 1
fi

if awk "BEGIN { exit !($coverage < $minimum_coverage) }"; then
  echo "Function parser coverage ${coverage}% is below the required ${minimum_coverage}%" >&2
  exit 1
fi

echo "Function parser coverage: ${coverage}% (target: 95%+)"
