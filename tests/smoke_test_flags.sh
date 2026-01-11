#!/bin/bash
# Smoke test for flag handling - verifies global flags, parent flags, and custom command inheritance
# This test uses the quick-start-advanced example which has custom commands that define --stack

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXAMPLE_DIR="$PROJECT_ROOT/examples/quick-start-advanced"

# Build atmos if needed
echo "=== Building atmos ==="
cd "$PROJECT_ROOT"
go build -o ./build/atmos .
ATMOS="$PROJECT_ROOT/build/atmos"

# Helper function
run_test() {
    local name="$1"
    local cmd="$2"
    echo ""
    echo "--- Test: $name ---"
    echo "Command: $cmd"
    if eval "$cmd" > /dev/null 2>&1; then
        echo "✓ PASS"
        return 0
    else
        echo "✗ FAIL"
        echo "Output:"
        eval "$cmd" 2>&1 || true
        return 1
    fi
}

run_test_output() {
    local name="$1"
    local cmd="$2"
    local expected="$3"
    echo ""
    echo "--- Test: $name ---"
    echo "Command: $cmd"
    local output
    output=$(eval "$cmd" 2>&1) || true
    if echo "$output" | grep -q "$expected"; then
        echo "✓ PASS (found: $expected)"
        return 0
    else
        echo "✗ FAIL (expected to find: $expected)"
        echo "Output:"
        echo "$output"
        return 1
    fi
}

cd "$EXAMPLE_DIR"
export ATMOS_CLI_CONFIG_PATH="$EXAMPLE_DIR"
export ATMOS_BASE_PATH="$EXAMPLE_DIR"

PASSED=0
FAILED=0

echo ""
echo "============================================"
echo "SMOKE TEST: Flag Handling Regression Tests"
echo "============================================"
echo "Working directory: $EXAMPLE_DIR"
echo ""

# ===========================================
# TEST GROUP 1: Global flags still work
# ===========================================
echo ""
echo "=== GROUP 1: Global Flags ==="

if run_test "atmos --help (global flag)" "$ATMOS --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "atmos version (basic command)" "$ATMOS version"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "atmos --version (global flag)" "$ATMOS --version"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "atmos describe stacks --help" "$ATMOS describe stacks --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# TEST GROUP 2: Terraform commands with --stack
# ===========================================
echo ""
echo "=== GROUP 2: Terraform Commands with --stack ==="

if run_test "atmos terraform --help" "$ATMOS terraform --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test_output "terraform help shows --stack flag" "$ATMOS terraform --help" "stack"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# TEST GROUP 3: Describe component (uses --stack)
# ===========================================
echo ""
echo "=== GROUP 3: Describe Component ==="

if run_test "describe component with -s flag" "$ATMOS describe component vpc -s plat-ue2-dev"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "describe component with --stack flag" "$ATMOS describe component vpc --stack plat-ue2-dev"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "describe component with --provenance" "$ATMOS describe component vpc-flow-logs-bucket -s plat-ue2-dev --provenance"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# TEST GROUP 4: Custom commands that define --stack (KEY TEST!)
# These commands define their own --stack flag which should now inherit from terraform
# ===========================================
echo ""
echo "=== GROUP 4: Custom Commands with --stack (Regression Test) ==="

if run_test "custom 'tf plan' command --help" "$ATMOS tf plan --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test_output "tf plan help shows --stack flag" "$ATMOS tf plan --help" "stack"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "custom 'terraform provision' command --help" "$ATMOS terraform provision --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test_output "terraform provision help shows --stack flag" "$ATMOS terraform provision --help" "stack"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "custom 'show component' command --help" "$ATMOS show component --help"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test_output "show component help shows --stack flag" "$ATMOS show component --help" "stack"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# TEST GROUP 5: List commands
# ===========================================
echo ""
echo "=== GROUP 5: List Commands ==="

if run_test "atmos list stacks" "$ATMOS list stacks"; then
    ((PASSED++))
else
    ((FAILED++))
fi

if run_test "atmos list components" "$ATMOS list components"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# TEST GROUP 6: Describe stacks
# ===========================================
echo ""
echo "=== GROUP 6: Describe Stacks ==="

if run_test "atmos describe stacks" "$ATMOS describe stacks --stack plat-ue2-dev"; then
    ((PASSED++))
else
    ((FAILED++))
fi

# ===========================================
# SUMMARY
# ===========================================
echo ""
echo "============================================"
echo "SMOKE TEST SUMMARY"
echo "============================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo "✓ ALL TESTS PASSED"
    exit 0
else
    echo "✗ SOME TESTS FAILED"
    exit 1
fi
