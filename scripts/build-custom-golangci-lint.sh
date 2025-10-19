#!/usr/bin/env bash
# Build custom golangci-lint binary with lintroller plugin.
# This creates ./custom-gcl binary in the project root.

set -e

echo "Building custom golangci-lint binary with lintroller plugin..."

# Build directly in project root using golangci-lint custom command.
# Disable VCS stamping to prevent issues when building.
GOFLAGS="-buildvcs=false" golangci-lint custom

echo "Custom golangci-lint binary built successfully: ./custom-gcl"
