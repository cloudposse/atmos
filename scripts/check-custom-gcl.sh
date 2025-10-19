#!/usr/bin/env bash
# Check if custom-gcl binary exists before running golangci-lint

if [[ ! -x ./custom-gcl ]]; then
    echo "Error: custom-gcl binary not found. Please run: make custom-gcl" >&2
    exit 1
fi

./custom-gcl run --new-from-rev=origin/main --config=.golangci.yml
