package markdown

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// WriteContent writes the full markdown content for test results.
// This is the main function that orchestrates writing a complete test summary.
func WriteContent(output io.Writer, summary *types.TestSummary, format string) {
	// Add UUID magic comment to prevent duplicate GitHub comments.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if uuid := os.Getenv("GOTCHA_COMMENT_UUID"); uuid != "" {
		fmt.Fprintf(output, "<!-- test-summary-uuid: %s -->\n\n", uuid)
	}

	// Add timestamp for local GitHub format runs.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if format == constants.FormatGitHub && os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// Test Results section (h1).
	fmt.Fprintf(output, "# Test Results\n\n")

	// Get test counts.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	// Display test results as shields.io badges - always show all badges.
	if total == 0 {
		fmt.Fprintf(output, "[![No Tests](https://shields.io/badge/NO_TESTS-0-inactive?style=for-the-badge)](#user-content-no-tests)")
	} else {
		fmt.Fprintf(output, "[![Passed](https://shields.io/badge/PASSED-%d-success?style=for-the-badge)](#user-content-passed) ", len(summary.Passed))
		fmt.Fprintf(output, "[![Failed](https://shields.io/badge/FAILED-%d-critical?style=for-the-badge)](#user-content-failed) ", len(summary.Failed))
		fmt.Fprintf(output, "[![Skipped](https://shields.io/badge/SKIPPED-%d-inactive?style=for-the-badge)](#user-content-skipped) ", len(summary.Skipped))
	}
	fmt.Fprintf(output, "\n\n")

	// Write test sections.
	WriteFailedTestsTable(output, summary.Failed)
	WriteSkippedTestsTable(output, summary.Skipped)
	WritePassedTestsTable(output, summary.Passed)

	// Test Coverage section (h1) - moved after test results.
	if summary.CoverageData != nil {
		WriteDetailedCoverage(output, summary.CoverageData)
	} else if summary.Coverage != "" {
		WriteBasicCoverage(output, summary.Coverage)
	}
}
