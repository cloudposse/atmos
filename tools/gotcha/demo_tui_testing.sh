#!/bin/bash
# Demo script showing how to test gotcha TUI mode in headless environments

echo "=== Gotcha TUI Testing Demo for AI/Headless Environments ==="
echo ""
echo "This script demonstrates how to test gotcha's TUI mode without a real TTY."
echo "This is useful for AI agents, CI/CD pipelines, and automated testing."
echo ""

# Method 1: Using GOTCHA_TEST_MODE with WithoutRenderer
echo "Method 1: Testing with GOTCHA_TEST_MODE=true (WithoutRenderer)"
echo "This uses Bubble Tea's WithoutRenderer option to run TUI logic without terminal rendering"
echo "Command: GOTCHA_TEST_MODE=true GOTCHA_FORCE_TUI=true ./gotcha stream ./test --show=all"
echo ""
GOTCHA_TEST_MODE=true GOTCHA_FORCE_TUI=true ./gotcha stream ./test --show=all 2>&1 | head -15
echo ""

# Method 2: Using normal stream mode (fallback)
echo "Method 2: Normal stream mode (when no TTY is available)"
echo "This is the default behavior when running without a TTY"
echo "Command: ./gotcha stream ./test --show=all"
echo ""
./gotcha stream ./test --show=all 2>&1 | head -15
echo ""

# Method 3: Programmatic testing with teatest
echo "Method 3: Programmatic testing with teatest library"
echo "This allows unit testing of TUI components without a TTY"
echo "Command: go test ./test -run TestTUIWithTeatest -v"
echo ""
go test ./test -run TestTUIWithTeatest -v 2>&1 | grep -E "(PASS|FAIL|RUN)" | head -10
echo ""

echo "=== Summary ==="
echo "• GOTCHA_TEST_MODE=true enables TUI testing in headless environments"
echo "• The teatest library allows programmatic testing of Bubble Tea apps"
echo "• Stream mode provides a fallback for non-TTY environments"
echo "• All methods work without requiring a real terminal"
