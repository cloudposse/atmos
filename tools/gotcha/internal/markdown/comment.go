package markdown

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// CommentSizeLimit represents GitHub's comment size limit.
const CommentSizeLimit = 65536

// generateCommentContent creates markdown content specifically for GitHub comments.
// It strategically resizes content to fit within GitHub's size limits while preserving
// the core intent: show failed & skipped tests and keep badges.
// GenerateGitHubComment creates markdown content for GitHub PR comments.
func GenerateGitHubComment(summary *types.TestSummary, uuid string) string {
	var content bytes.Buffer

	// Add UUID magic comment to prevent duplicate GitHub comments.
	if uuid != "" {
		fmt.Fprintf(&content, "<!-- test-summary-uuid: %s -->\n\n", uuid)
	}

	// Test Results section (h1).
	fmt.Fprintf(&content, "# Test Results\n\n")

	// Get test counts.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	// Display test results as shields.io badges - always show all badges.
	if total == 0 {
		fmt.Fprintf(&content, "[![No Tests](https://shields.io/badge/NO_TESTS-0-inactive?style=for-the-badge)](#user-content-no-tests)")
	} else {
		fmt.Fprintf(&content, "[![Passed](https://shields.io/badge/PASSED-%d-success?style=for-the-badge)](#user-content-passed) ", len(summary.Passed))
		fmt.Fprintf(&content, "[![Failed](https://shields.io/badge/FAILED-%d-critical?style=for-the-badge)](#user-content-failed) ", len(summary.Failed))
		fmt.Fprintf(&content, "[![Skipped](https://shields.io/badge/SKIPPED-%d-inactive?style=for-the-badge)](#user-content-skipped) ", len(summary.Skipped))
	}
	fmt.Fprintf(&content, "\n\n")

	// Check if current content size is already too large for a basic comment.
	// If not, try to add full sections and strategically trim if needed.
	currentSize := content.Len()

	// Always show failed and skipped tests (these are most important) - but use compact format.
	writeCompactFailedTests(&content, summary.Failed)
	writeCompactSkippedTests(&content, summary.Skipped)

	currentSize = content.Len()

	// If we're already over the limit with just failed/skipped tests,
	// we have a more serious problem and may need to truncate those too.
	if currentSize > CommentSizeLimit {
		return truncateToEssentials(summary, uuid)
	}

	// Don't add passed tests to comments - they're only for job summaries.
	// GitHub comments should focus on failures, skips, and basic coverage only.

	// Try to add coverage if there's still room.
	currentSize = content.Len()
	if currentSize < CommentSizeLimit {
		remainingBytes := CommentSizeLimit - currentSize
		addCoverageWithLimit(&content, summary, remainingBytes)
	}

	result := content.String()

	// Final safety check - if we're still over the limit, do basic truncation.
	if len(result) > CommentSizeLimit {
		truncationMsg := "\n\n---\n*Comment truncated due to size limits. See full results in job summary.*"
		availableSize := CommentSizeLimit - len(truncationMsg)

		if availableSize <= 0 {
			return truncationMsg
		}

		// Try to truncate at a reasonable boundary (line break).
		truncated := result[:availableSize]
		if lastNewline := bytes.LastIndexByte([]byte(truncated), '\n'); lastNewline > availableSize/2 {
			truncated = truncated[:lastNewline]
		}

		return truncated + truncationMsg
	}

	return result
}

// truncateToEssentials creates a minimal comment with only the most critical information.
func truncateToEssentials(summary *types.TestSummary, uuid string) string {
	var content bytes.Buffer

	// Add UUID magic comment.
	if uuid != "" {
		fmt.Fprintf(&content, "<!-- test-summary-uuid: %s -->\n\n", uuid)
	}

	// Test Results section (h1).
	fmt.Fprintf(&content, "# Test Results\n\n")

	// Get test counts.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	// Display test results as shields.io badges - always show all badges.
	if total == 0 {
		fmt.Fprintf(&content, "[![No Tests](https://shields.io/badge/NO_TESTS-0-inactive?style=for-the-badge)](#user-content-no-tests)\n\n")
	} else {
		fmt.Fprintf(&content, "[![Passed](https://shields.io/badge/PASSED-%d-success?style=for-the-badge)](#user-content-passed) ", len(summary.Passed))
		fmt.Fprintf(&content, "[![Failed](https://shields.io/badge/FAILED-%d-critical?style=for-the-badge)](#user-content-failed) ", len(summary.Failed))
		fmt.Fprintf(&content, "[![Skipped](https://shields.io/badge/SKIPPED-%d-inactive?style=for-the-badge)](#user-content-skipped)\n\n", len(summary.Skipped))
	}

	// Show only a limited number of failed tests if any.
	if len(summary.Failed) > 0 {
		maxFailed := 10 // Show at most 10 failed tests.
		if len(summary.Failed) > maxFailed {
			fmt.Fprintf(&content, "### ❌ Failed Tests (%d, showing first %d)\n\n", len(summary.Failed), maxFailed)
		} else {
			fmt.Fprintf(&content, "### ❌ Failed Tests (%d)\n\n", len(summary.Failed))
		}

		fmt.Fprintf(&content, "| Test | Package |\n|------|--------|\n")
		for i, test := range summary.Failed {
			if i >= maxFailed {
				break
			}
			pkg := types.ShortPackage(test.Package)
			fmt.Fprintf(&content, "| `%s` | %s |\n", test.Test, pkg)
		}
		fmt.Fprintf(&content, "\n")
	}

	// Show only a limited number of skipped tests if any.
	if len(summary.Skipped) > 0 {
		maxSkipped := 5 // Show at most 5 skipped tests.
		if len(summary.Skipped) > maxSkipped {
			fmt.Fprintf(&content, "### ⏭️ Skipped Tests (%d, showing first %d)\n\n", len(summary.Skipped), maxSkipped)
		} else {
			fmt.Fprintf(&content, "### ⏭️ Skipped Tests (%d)\n\n", len(summary.Skipped))
		}

		fmt.Fprintf(&content, "| Test | Package |\n|------|--------|\n")
		for i, test := range summary.Skipped {
			if i >= maxSkipped {
				break
			}
			pkg := types.ShortPackage(test.Package)
			fmt.Fprintf(&content, "| `%s` | %s |\n", test.Test, pkg)
		}
		fmt.Fprintf(&content, "\n")
	}

	fmt.Fprintf(&content, "_Full test results available in job summary._\n")

	return content.String()
}

// addPassedTestsWithLimit adds passed tests section with intelligent size limiting.
func addPassedTestsWithLimit(output io.Writer, passed []types.TestResult, maxBytes int) {
	if len(passed) == 0 || maxBytes < 500 { // Need at least 500 bytes for a meaningful section.
		return
	}

	// Estimate bytes needed for header and basic structure.
	headerBytes := 200 // Rough estimate for section header.
	if maxBytes < headerBytes {
		return
	}

	availableBytes := maxBytes - headerBytes

	// Calculate how many tests we can show based on average test entry size.
	// Each test entry is roughly: "| `TestName` | package | 1.23s | 5.0% |\n" ~50-80 bytes.
	avgTestEntryBytes := 70
	maxTests := availableBytes / avgTestEntryBytes

	if maxTests <= 0 {
		return
	}

	// Sort passed tests by duration (slowest first) and take the fastest ones.
	sortedPassed := make([]types.TestResult, len(passed))
	copy(sortedPassed, passed)
	sort.Slice(sortedPassed, func(i, j int) bool {
		return sortedPassed[i].Duration < sortedPassed[j].Duration // Fastest first.
	})

	// Limit to the fastest tests that fit.
	displayTests := sortedPassed
	if len(displayTests) > maxTests {
		displayTests = sortedPassed[:maxTests]
	}

	fmt.Fprintf(output, "### ✅ Passed Tests (%d, showing %d fastest)\n\n", len(passed), len(displayTests))
	fmt.Fprintf(output, "| Test | Package | Duration |\n|------|---------|----------|\n")

	for _, test := range displayTests {
		pkg := types.ShortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
	}
	fmt.Fprintf(output, "\n")
}

// addCoverageWithLimit adds coverage information if there's enough space using job summary format.
func addCoverageWithLimit(output io.Writer, summary *types.TestSummary, maxBytes int) {
	if maxBytes < 200 { // Need at least 200 bytes for coverage table format.
		return
	}

	// Use the same table format as job summary.
	if summary.CoverageData != nil && summary.CoverageData.StatementCoverage != "" {
		fmt.Fprintf(output, "## 📊 Test Coverage\n\n")

		// Build statement coverage details with emoji.
		coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(summary.CoverageData.StatementCoverage, "%"), 64)
		statementEmoji := getCoverageEmoji(coverageFloat)

		statementDetails := statementEmoji
		if len(summary.CoverageData.FilteredFiles) > 0 {
			statementDetails += fmt.Sprintf(" (excluded %d mock files)", len(summary.CoverageData.FilteredFiles))
		}

		// Calculate function coverage statistics.
		coveredFunctions, totalFunctions, functionCoveragePercent := calculateFunctionCoverage(summary.CoverageData.FunctionCoverage)
		funcEmoji := getCoverageEmoji(functionCoveragePercent)
		functionDetails := fmt.Sprintf("%s %d/%d functions covered", funcEmoji, coveredFunctions, totalFunctions)

		// Write coverage table using same format as job summary.
		fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
		fmt.Fprintf(output, "|--------|----------|----------|\n")
		fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n", summary.CoverageData.StatementCoverage, statementDetails)
		fmt.Fprintf(output, "| Function Coverage | %.1f%% | %s |\n\n", functionCoveragePercent, functionDetails)

	} else if summary.Coverage != "" {
		fmt.Fprintf(output, "## 📊 Test Coverage\n\n")

		// Legacy format with emoji.
		coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(summary.Coverage, "%"), 64)
		emoji := getCoverageEmoji(coverageFloat)

		fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
		fmt.Fprintf(output, "|--------|----------|----------|\n")
		fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n\n", summary.Coverage, emoji)
	}
}

// writeCompactFailedTests writes a compact failed tests section for GitHub comments.
func writeCompactFailedTests(output io.Writer, failed []types.TestResult) {
	if len(failed) == 0 {
		return // Hide entire section when no failures
	}

	fmt.Fprintf(output, "### ❌ Failed Tests (%d)\n\n", len(failed))
	fmt.Fprintf(output, "<details>\n<summary>Click to see failed tests</summary>\n\n")

	for _, test := range failed {
		pkg := types.ShortPackage(test.Package)
		fmt.Fprintf(output, "- `%s` in %s (%.2fs)\n", test.Test, pkg, test.Duration)
	}

	fmt.Fprintf(output, "\n</details>\n\n")
}

// writeCompactSkippedTests writes a compact skipped tests section for GitHub comments.
func writeCompactSkippedTests(output io.Writer, skipped []types.TestResult) {
	if len(skipped) == 0 {
		return
	}

	fmt.Fprintf(output, "### ⏭️ Skipped Tests (%d)\n\n", len(skipped))
	fmt.Fprintf(output, "<details>\n<summary>Click to see skipped tests</summary>\n\n")

	for _, test := range skipped {
		pkg := types.ShortPackage(test.Package)
		fmt.Fprintf(output, "- `%s` in %s\n", test.Test, pkg)
	}

	fmt.Fprintf(output, "\n</details>\n\n")
}
