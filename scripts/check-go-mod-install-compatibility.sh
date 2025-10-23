#!/usr/bin/env bash
# Check that go.mod doesn't contain replace or exclude directives.
# These directives break `go install github.com/cloudposse/atmos@latest`.

set -e

GO_MOD_FILE="go.mod"

if [ ! -f "$GO_MOD_FILE" ]; then
    echo "Error: go.mod not found"
    exit 1
fi

# Check for replace directives.
if grep -E "^replace\s+|^replace\s*\(" "$GO_MOD_FILE" > /dev/null 2>&1; then
    echo "ERROR: go.mod contains 'replace' directives which break 'go install'."
    echo ""
    echo "Replace directives found:"
    grep -E "^replace\s+|^replace\s*\(|^\s+.*=>.*" "$GO_MOD_FILE" | grep -v "^//"
    echo ""
    echo "This breaks a documented installation method for Atmos."
    echo "Consider alternative approaches that don't break go install compatibility."
    exit 1
fi

# Check for exclude directives.
if grep -E "^exclude\s+|^exclude\s*\(" "$GO_MOD_FILE" > /dev/null 2>&1; then
    echo "ERROR: go.mod contains 'exclude' directives which break 'go install'."
    echo ""
    echo "Exclude directives found:"
    grep -E "^exclude\s+|^exclude\s*\(|^\s+[a-z]" "$GO_MOD_FILE" | grep -v "^//"
    echo ""
    echo "This breaks a documented installation method for Atmos."
    echo "Consider alternative approaches that don't break go install compatibility."
    exit 1
fi

echo "✓ go.mod is compatible with 'go install'"
exit 0
