package markdown

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// WriteContent writes the full markdown content for test results.
// This is the main function that orchestrates writing a complete test summary.
func WriteContent(output io.Writer, summary *types.TestSummary, format string) {
	// Add UUID magic comment to prevent duplicate GitHub comments.
	uuid := config.GetCommentUUID()
	if uuid != "" {
		fmt.Fprintf(output, "<!-- test-summary-uuid: %s -->\n\n", uuid)
	}

	// Add timestamp for local GitHub format runs.
	if format == constants.FormatGitHub && config.GetGitHubStepSummary() == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// Extract discriminator from UUID if available
	// UUID format is typically "project-context-gotcha-platform" 
	// We want to extract "gotcha/platform" as the discriminator
	discriminator := ""
	if uuid != "" && strings.Contains(uuid, "gotcha-") {
		parts := strings.Split(uuid, "gotcha-")
		if len(parts) > 1 {
			discriminator = "gotcha/" + parts[1]
		}
	}

	// Determine status emoji based on test results
	statusEmoji := "✅" // Default to pass
	if len(summary.Failed) > 0 {
		statusEmoji = "❌"
	}

	// Test Results section (h1) with optional discriminator and emoji.
	if discriminator != "" {
		fmt.Fprintf(output, "# %s Test Results (%s)\n\n", statusEmoji, discriminator)
	} else {
		fmt.Fprintf(output, "# Test Results\n\n")
	}

	// Display total elapsed time if available
	if summary.TotalElapsedTime > 0 {
		fmt.Fprintf(output, "_Total Time: %.2fs_\n\n", summary.TotalElapsedTime)
	}

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
