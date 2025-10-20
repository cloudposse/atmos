#!/usr/bin/env bash
# Run custom golangci-lint binary with lintroller plugin.
# The binary must be built first using: make custom-gcl

set -e

if [[ ! -x ./custom-gcl ]]; then
    echo "Error: custom-gcl binary not found." >&2
    echo "Please build it first by running: make custom-gcl" >&2
    exit 1
fi

./custom-gcl run --new-from-rev=origin/main --config=.golangci.yml
