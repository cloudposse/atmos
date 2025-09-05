package parser

import (
	"strings"
)

// shouldShowErrorLine determines if a line contains useful error information.
// ShouldShowErrorLine determines if an error line should be displayed.
func ShouldShowErrorLine(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Skip the RUN/PASS/FAIL status lines
	if strings.HasPrefix(trimmed, "=== RUN") ||
		strings.HasPrefix(trimmed, "=== PAUSE") ||
		strings.HasPrefix(trimmed, "=== CONT") {
		return false
	}

	// Skip the --- PASS/FAIL lines (we show our own summary)
	if strings.HasPrefix(trimmed, "--- PASS") ||
		strings.HasPrefix(trimmed, "--- FAIL") ||
		strings.HasPrefix(trimmed, "--- SKIP") {
		return false
	}

	// Show actual error messages
	if strings.Contains(line, "_test.go:") || // File:line references
		strings.Contains(line, "Error:") ||
		strings.Contains(line, "Error Trace:") ||
		strings.Contains(line, "Test:") ||
		strings.Contains(line, "Messages:") ||
		strings.Contains(line, "expected:") ||
		strings.Contains(line, "actual:") ||
		strings.Contains(line, "got:") ||
		strings.Contains(line, "want:") {
		return true
	}

	// Show assertion failures
	if strings.Contains(line, "assertion failed") ||
		strings.Contains(line, "should be") ||
		strings.Contains(line, "should have") ||
		strings.Contains(line, "expected") {
		return true
	}

	// Skip empty lines in error output
	if trimmed == "" {
		return false
	}

	// When in doubt, show it if it's indented (part of test output)
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}
