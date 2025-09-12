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

	// Show actual error messages (case-insensitive for some patterns)
	lowerLine := strings.ToLower(trimmed)
	if strings.Contains(line, "_test.go:") &&
		(strings.Contains(line, "Error") || strings.Contains(line, "Failed") || strings.Contains(line, "assertion")) || // Test file errors
		strings.Contains(line, ".go:") && strings.Contains(lowerLine, "error") || // Go file compilation errors like "./main.go:10:1: syntax error"
		strings.Contains(line, "Error:") ||
		strings.Contains(line, "Error Trace:") ||
		strings.Contains(line, "Test:") ||
		strings.Contains(line, "Messages:") ||
		strings.Contains(line, "expected:") ||
		strings.Contains(line, "actual:") ||
		strings.Contains(line, "got:") ||
		strings.Contains(line, "want:") ||
		strings.HasPrefix(lowerLine, "fail") || // FAIL package lines (with tab or space)
		strings.HasPrefix(trimmed, "#") || // Compilation errors start with #
		strings.Contains(lowerLine, "error:") ||
		strings.Contains(lowerLine, "warning:") ||
		strings.Contains(lowerLine, "warn:") ||
		strings.Contains(lowerLine, "panic:") ||
		strings.Contains(lowerLine, "compilation error") ||
		strings.Contains(lowerLine, "build error") ||
		strings.Contains(lowerLine, "race condition") ||
		strings.Contains(lowerLine, "data race") {
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

	// Don't show indented lines by default - they're usually normal test output
	// The error patterns above will catch actual errors
	return false
}
