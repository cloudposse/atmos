#!/usr/bin/env bash
set -e

echo "Building custom golangci-lint binary with lintroller plugin..."

# Create temporary directory for isolated build
TMPDIR=$(mktemp -d)
echo "Using temporary build directory: $TMPDIR"

# Copy required files to temporary directory
cp .custom-gcl.yml "$TMPDIR/"
cp -r tools "$TMPDIR/"

# Build in temporary directory
cd "$TMPDIR"
golangci-lint custom

# Move binary back to original directory
mv ./custom-gcl "$OLDPWD/custom-gcl"

# Clean up
cd "$OLDPWD"
rm -rf "$TMPDIR"

echo "Custom golangci-lint binary built successfully"
