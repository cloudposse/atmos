package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

// Size constants.
const (
	// BytesPerKB is the number of bytes in a kilobyte.
	BytesPerKB = 1024.0
	// PercentageMultiplier for converting fractions to percentages.
	PercentageMultiplier = 100
)

// Filter values for test display.
const (
	FilterAll       = "all"
	FilterFailed    = "failed"
	FilterPassed    = "passed"
	FilterSkipped   = "skipped"
	FilterCollapsed = "collapsed"
	FilterNone      = "none"
)

// Display constants.
const (
	MaxDotsInProgress = 10 // Maximum number of dots to show in progress indicator
	SingleItemCount   = 1  // Used for pluralization checks
)

// shouldShowTest determines if a test should be displayed based on filter.
func (m *TestModel) shouldShowTest(status string) bool {
	switch m.showFilter {
	case FilterAll:
		return true
	case FilterFailed:
		// Show both failed and skipped tests when filter is "failed"
		return status == TestStatusFail || status == TestStatusSkip
	case FilterPassed:
		return status == TestStatusPass
	case FilterSkipped:
		return status == TestStatusSkip
	case FilterCollapsed:
		return status == TestStatusFail // Only show failures in collapsed mode
	case FilterNone:
		return false
	default:
		return true
	}
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (m *TestModel) generateSubtestProgress(passed, total int) string {
	if total == 0 {
		return ""
	}

	// If no tests passed, return empty string (no progress to show)
	if passed == 0 {
		return ""
	}

	// Calculate how many dots for passed vs failed
	passedDots := passed
	failedDots := total - passed

	// If we need to scale down to MaxDotsInProgress, do it proportionally
	if total > MaxDotsInProgress {
		passedDots = (passed * MaxDotsInProgress) / total
		failedDots = MaxDotsInProgress - passedDots
	}

	// Build the indicator with colored dots
	var indicator strings.Builder

	// Add green dots for passed tests
	for i := 0; i < passedDots; i++ {
		indicator.WriteString(PassStyle.Render("●"))
	}

	// Add red dots for failed tests
	for i := 0; i < failedDots; i++ {
		indicator.WriteString(FailStyle.Render("●"))
	}

	return indicator.String()
}

// pluralize returns "s" if count is not 1, otherwise empty string.
func pluralize(count int) string {
	if count == SingleItemCount {
		return ""
	}
	return "s"
}

// GenerateFinalSummary generates the final summary display.
func (m *TestModel) GenerateFinalSummary() string {
	var summary strings.Builder

	// Generate test summary
	totalTests := m.passCount + m.failCount + m.skipCount
	summary.WriteString(fmt.Sprintf("\n%s\n", StatsHeaderStyle.Render("Test Results:")))
	summary.WriteString(fmt.Sprintf("  %s Passed:  %5d\n", PassStyle.Render(CheckPass), m.passCount))
	summary.WriteString(fmt.Sprintf("  %s Failed:  %5d\n", FailStyle.Render(CheckFail), m.failCount))
	summary.WriteString(fmt.Sprintf("  %s Skipped: %5d\n", SkipStyle.Render(CheckSkip), m.skipCount))
	summary.WriteString(fmt.Sprintf("  Total:     %5d\n", totalTests))

	// Add coverage summary if available
	packagesWithStatementCoverage := 0
	packagesWithFunctionCoverage := 0
	totalStatementCoverage := 0.0
	totalFunctionCoverage := 0.0

	for _, pkg := range m.packageResults {
		// Try statement coverage first
		if pkg.StatementCoverage != "" && pkg.StatementCoverage != "0.0%" && pkg.StatementCoverage != "N/A" {
			packagesWithStatementCoverage++
			var pct float64
			if _, err := fmt.Sscanf(pkg.StatementCoverage, "%f%%", &pct); err == nil {
				totalStatementCoverage += pct
			}
		} else if pkg.Coverage != "" && pkg.Coverage != "0.0%" {
			// Fallback to legacy coverage
			packagesWithStatementCoverage++
			var pct float64
			if _, err := fmt.Sscanf(pkg.Coverage, "%f%%", &pct); err == nil {
				totalStatementCoverage += pct
			}
		}

		// Check function coverage
		if pkg.FunctionCoverage != "" && pkg.FunctionCoverage != "0.0%" && pkg.FunctionCoverage != "N/A" {
			packagesWithFunctionCoverage++
			var pct float64
			if _, err := fmt.Sscanf(pkg.FunctionCoverage, "%f%%", &pct); err == nil {
				totalFunctionCoverage += pct
			}
		}
	}

	// Display coverage based on what's available
	if packagesWithStatementCoverage > 0 {
		avgStatementCoverage := totalStatementCoverage / float64(packagesWithStatementCoverage)

		if packagesWithFunctionCoverage > 0 {
			avgFunctionCoverage := totalFunctionCoverage / float64(packagesWithFunctionCoverage)
			// Show both types
			summary.WriteString(fmt.Sprintf("  Statement Coverage: %5.1f%%\n", avgStatementCoverage))
			summary.WriteString(fmt.Sprintf("  Function Coverage:  %5.1f%%\n", avgFunctionCoverage))
		} else {
			// Only statement coverage available
			summary.WriteString(fmt.Sprintf("  Statement Coverage: %5.1f%%\n", avgStatementCoverage))
		}
	}

	// Log completion time
	elapsed := time.Since(m.startTime)
	summary.WriteString(fmt.Sprintf("\n%s Tests completed in %.2fs\n", DurationStyle.Render("ℹ"), elapsed.Seconds()))

	// Add exit status information
	exitCode := m.GetExitCode()
	switch {
	case m.aborted:
		summary.WriteString(fmt.Sprintf("\n%s Test run aborted (exit code %d)\n", FailStyle.Render("✗"), exitCode))
	case m.failCount > 0:
		summary.WriteString(fmt.Sprintf("\n%s %d test%s failed (exit code %d)\n",
			FailStyle.Render("✗"),
			m.failCount,
			pluralize(m.failCount),
			exitCode))
	case totalTests == 0:
		summary.WriteString(fmt.Sprintf("\n%s No tests found (exit code %d)\n", SkipStyle.Render("⚠"), exitCode))
	default:
		summary.WriteString(fmt.Sprintf("\n%s All tests passed (exit code %d)\n", PassStyle.Render("✓"), exitCode))
	}

	return summary.String()
}

// getBufferSizeKB returns the current buffer size in KB.
func (m *TestModel) getBufferSizeKB() float64 {
	totalSize := 0

	// Calculate size of output buffer
	totalSize += len(m.outputBuffer)

	// Calculate size of all test buffers
	for _, buffer := range m.buffers {
		for _, line := range buffer {
			totalSize += len(line)
		}
	}

	// Calculate size of package results
	for _, pkg := range m.packageResults {
		// Package output
		for _, line := range pkg.Output {
			totalSize += len(line)
		}

		// Test output
		for _, test := range pkg.Tests {
			for _, line := range test.Output {
				totalSize += len(line)
			}
			// Subtest output
			for _, subtest := range test.Subtests {
				for _, line := range subtest.Output {
					totalSize += len(line)
				}
			}
		}
	}

	// Convert to KB
	return float64(totalSize) / BytesPerKB
}

// emitAlert outputs a terminal bell if alert is enabled.
func emitAlert(enabled bool) {
	if enabled {
		fmt.Fprint(os.Stderr, "\a")
	}
}

// displayPackageResult generates the display string for a package result.
//
//nolint:nestif,gocognit // Complex package display logic with multiple decision points:
// - Build failure vs test failure differentiation
// - No-test-files handling
// - Filter-based test visibility (all, failed, passed, skipped, collapsed)
// - Test hierarchy display with proper indentation
// - Coverage information formatting
// - Package status determination from test results
// The nesting is essential for proper test result visualization.
func (m *TestModel) displayPackageResult(pkg *PackageResult) string {
	var output strings.Builder

	// Package header
	// Display package header - ▶ icon in white, package name in cyan
	output.WriteString(fmt.Sprintf("▶ %s\n", PackageHeaderStyle.Render(pkg.Package)))

	// Check for "No tests"
	// Check for package-level failures (e.g., TestMain failures)
	if pkg.Status == TestStatusFail && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely TestMain failure)
		output.WriteString(fmt.Sprintf("\n  %s Package failed to run tests\n", FailStyle.Render(CheckFail)))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					output.WriteString(fmt.Sprintf("    %s", line))
				}
			}
		}
		return output.String()
	}

	if pkg.Status == TestStatusSkip || m.packagesWithNoTests[pkg.Package] || !pkg.HasTests {
		// Show more specific message if a filter is applied
		if m.testFilter != "" {
			output.WriteString(fmt.Sprintf("\n  %s\n", DurationStyle.Render("No tests matching filter")))
		} else {
			output.WriteString(fmt.Sprintf("\n  %s\n", DurationStyle.Render("No tests")))
		}
		return output.String()
	}

	// Count test results for this package
	// Since subtests are now in pkg.Tests directly, just count all tests
	var passedCount, failedCount, skippedCount int
	for _, test := range pkg.Tests {
		switch test.Status {
		case TestStatusPass:
			passedCount++
		case TestStatusFail:
			failedCount++
		case TestStatusSkip:
			skippedCount++
		}
	}

	// Display tests in order (including subtests as individual entries)

	// Debug: Log test order details
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "[DISPLAY-DEBUG] Package %s: TestOrder has %d tests, Tests map has %d tests, showFilter=%s\n",
				pkg.Package, len(pkg.TestOrder), len(pkg.Tests), m.showFilter)
			for i, name := range pkg.TestOrder {
				fmt.Fprintf(f, "[DISPLAY-DEBUG]   TestOrder[%d]: %s\n", i, name)
			}
			f.Close()
		}
	}

	// Add blank line before tests section (if any tests will be displayed)
	firstTestCheck := false
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		if test != nil {
			shouldShow := m.shouldShowTest(test.Status)
			if shouldShow || m.showFilter == FilterCollapsed {
				if !firstTestCheck {
					output.WriteString(constants.NewlineString)
					firstTestCheck = true
				}
				break
			}
		}
	}

	// Debug: Log all tests in TestOrder
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "[DISPLAY-DEBUG] Package %s has %d tests in TestOrder\n", pkg.Package, len(pkg.TestOrder))
			for i, name := range pkg.TestOrder {
				test := pkg.Tests[name]
				if test != nil {
					fmt.Fprintf(f, "[DISPLAY-DEBUG]   [%d] %s: status=%s, parent=%s, subtests=%d\n",
						i, name, test.Status, test.Parent, len(test.Subtests))
				}
			}
			f.Close()
		}
	}

	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		if test == nil {
			continue
		}

		// Check if this is a subtest (has a parent)
		isSubtest := test.Parent != ""

		shouldShow := m.shouldShowTest(test.Status)
		if shouldShow || m.showFilter == FilterCollapsed {
			// For parent tests, use the full formatter to show mini indicators
			// For subtests, use the simple line display
			if isSubtest {
				m.displayTestAsLine(&output, test, "  ")
			} else {
				formatter := NewTestFormatter(m)
				formatter.FormatTest(&output, test)
			}
		}
	}

	// Always display summary line when package has tests
	// This ensures summaries are shown even when individual tests are filtered out
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		// Add spacing before summary only if tests were displayed
		if firstTestCheck {
			output.WriteString(constants.NewlineString)
		}

		var summaryLine string
		coverageStr := ""

		// Build coverage string with both statement and function coverage
		if pkg.StatementCoverage != "" && pkg.StatementCoverage != constants.ZeroPercentString {
			if pkg.FunctionCoverage != "" && pkg.FunctionCoverage != "N/A" {
				coverageStr = fmt.Sprintf(" (statements: %s, functions: %s)",
					pkg.StatementCoverage, pkg.FunctionCoverage)
			} else {
				// Only statement coverage available from standard Go test output
				coverageStr = fmt.Sprintf(" (%s coverage)", pkg.StatementCoverage)
			}
		} else if pkg.Coverage != "" {
			// Fallback to legacy coverage if new fields aren't set
			coverageStr = fmt.Sprintf(" (%s coverage)", pkg.Coverage)
		}

		switch {
		case failedCount > 0:
			// Show failure summary
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s",
				FailStyle.Render(CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		case passedCount > 0:
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s",
				PassStyle.Render(CheckPass),
				passedCount,
				coverageStr)
		case skippedCount > 0:
			// Only skipped tests
			testWord := "tests"
			if skippedCount == 1 {
				testWord = "test"
			}
			summaryLine = fmt.Sprintf("  %s %d %s skipped%s",
				SkipStyle.Render(CheckSkip),
				skippedCount,
				testWord,
				coverageStr)
		}

		if summaryLine != "" {
			output.WriteString(fmt.Sprintf("%s\n", summaryLine))
		}
	}

	return output.String()
}

// displayTestAsLine displays a test as a simple one-line entry.
func (m *TestModel) displayTestAsLine(output *strings.Builder, test *TestResult, indent string) {
	// Skip running tests
	if test.Status != TestStatusPass && test.Status != TestStatusFail && test.Status != TestStatusSkip {
		return
	}

	// Get the status icon
	var icon string
	switch test.Status {
	case TestStatusPass:
		icon = PassStyle.Render(CheckPass)
	case TestStatusFail:
		icon = FailStyle.Render(CheckFail)
	case TestStatusSkip:
		icon = SkipStyle.Render(CheckSkip)
	}

	// Format the line
	fmt.Fprintf(output, "  %s%s %s", indent, icon, TestNameStyle.Render(test.Name))

	// Add duration
	if test.Elapsed > 0 {
		duration := fmt.Sprintf("(%.2fs)", test.Elapsed)
		fmt.Fprintf(output, constants.SpaceFormatString, DurationStyle.Render(duration))
	}

	// Add skip reason if applicable
	if test.Status == TestStatusSkip && test.SkipReason != "" {
		reason := fmt.Sprintf("- %s", test.SkipReason)
		fmt.Fprintf(output, constants.SpaceFormatString, DurationStyle.Render(reason))
	}

	output.WriteString("\n")

	// Show output for failed tests if not collapsed
	if test.Status == TestStatusFail && m.showFilter != FilterCollapsed && len(test.Output) > 0 {
		output.WriteString(constants.NewlineString)
		formatter := func(line string) string {
			if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
				formatted := strings.ReplaceAll(line, `\t`, "\t")
				return strings.ReplaceAll(formatted, `\n`, "\n")
			}
			return line
		}

		for _, line := range test.Output {
			output.WriteString("    " + indent)
			output.WriteString(formatter(line))
		}
		output.WriteString(constants.NewlineString)
	}
}

// displayTestOld is the original implementation preserved for reference.
// TODO: Remove after verifying the refactored version works correctly.
func (m *TestModel) displayTestOld(output *strings.Builder, test *TestResult) {
	// Check if this test has subtests
	hasSubtests := len(test.Subtests) > 0

	// Build the test display
	var styledIcon string
	switch test.Status {
	case TestStatusPass:
		styledIcon = PassStyle.Render(CheckPass)
	case TestStatusFail:
		styledIcon = FailStyle.Render(CheckFail)
	case TestStatusSkip:
		styledIcon = SkipStyle.Render(CheckSkip)
	default:
		return // Don't display running tests
	}

	// Display the test
	fmt.Fprintf(output, "  %s %s", styledIcon, TestNameStyle.Render(test.Name))

	// Add duration if available
	if test.Elapsed > 0 {
		fmt.Fprintf(output, constants.SpaceFormatString, DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Add skip reason if available
	if test.Status == TestStatusSkip && test.SkipReason != "" {
		fmt.Fprintf(output, constants.SpaceFormatString, DurationStyle.Render(fmt.Sprintf("- %s", test.SkipReason)))
	}

	// Add subtest progress indicator if it has subtests
	if hasSubtests && m.subtestStats[test.Name] != nil {
		stats := m.subtestStats[test.Name]
		totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)

		if totalSubtests > 0 {
			miniProgress := m.generateSubtestProgress(len(stats.passed), totalSubtests)
			percentage := (len(stats.passed) * PercentageMultiplier) / totalSubtests
			fmt.Fprintf(output, " %s %d%% passed", miniProgress, percentage)
		}
	}

	output.WriteString("\n")

	// Show test output for failed tests if not in collapsed mode
	if test.Status == TestStatusFail && m.showFilter != FilterCollapsed && len(test.Output) > 0 {
		output.WriteString(constants.NewlineString)
		if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, line := range test.Output {
				// Replace literal \t with actual tabs and \n with newlines
				formatted := strings.ReplaceAll(line, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, constants.NewlineString)
				output.WriteString("    " + formatted)
			}
		} else {
			// Default: show output as-is
			for _, line := range test.Output {
				output.WriteString("    " + line)
			}
		}
		output.WriteString(constants.NewlineString)
	}

	// Show detailed subtest results for failed parent tests
	if test.Status == TestStatusFail && hasSubtests && m.showFilter != FilterCollapsed {
		stats := m.subtestStats[test.Name]
		if stats != nil {
			totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)
			if totalSubtests > 0 {
				fmt.Fprintf(output, "\n    Subtest Summary: %d passed, %d failed of %d total\n",
					len(stats.passed), len(stats.failed), totalSubtests)

				// Show passed subtests
				if len(stats.passed) > 0 {
					fmt.Fprintf(output, "\n    %s Passed (%d):\n", PassStyle.Render("✔"), len(stats.passed))
					for _, name := range stats.passed {
						// Extract just the subtest name, not the full path
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						fmt.Fprintf(output, "      • %s\n", subtestName)
					}
				}

				// Show failed subtests with their output
				if len(stats.failed) > 0 {
					fmt.Fprintf(output, "\n    %s Failed (%d):\n", FailStyle.Render("✘"), len(stats.failed))
					for _, name := range stats.failed {
						// Extract just the subtest name
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						fmt.Fprintf(output, "      • %s\n", subtestName)

						// Show subtest output if available
						if subtest := test.Subtests[name]; subtest != nil && len(subtest.Output) > 0 {
							if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
								// With full output, properly render tabs and maintain formatting
								for _, line := range subtest.Output {
									formatted := strings.ReplaceAll(line, `\t`, "\t")
									formatted = strings.ReplaceAll(formatted, `\n`, constants.NewlineString)
									output.WriteString("        " + formatted)
								}
							} else {
								for _, line := range subtest.Output {
									output.WriteString("        " + line)
								}
							}
						}
					}
				}

				// Show skipped subtests if any
				if len(stats.skipped) > 0 {
					fmt.Fprintf(output, "\n    %s Skipped (%d):\n", SkipStyle.Render("⊘"), len(stats.skipped))
					for _, name := range stats.skipped {
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						fmt.Fprintf(output, "      • %s\n", subtestName)
					}
				}
			}
		}
	}
}
