package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// shouldShowTest determines if a test should be displayed based on filter.
func (m *TestModel) shouldShowTest(status string) bool {
	switch m.showFilter {
	case "all":
		return true
	case "failed":
		// Show both failed and skipped tests when filter is "failed"
		return status == "fail" || status == "skip"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	case "collapsed":
		return status == "fail" // Only show failures in collapsed mode
	case "none":
		return false
	default:
		return true
	}
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (m *TestModel) generateSubtestProgress(passed, total int) string {
	const maxDots = 10 // Maximum number of dots to show for readability

	if total == 0 {
		return ""
	}

	// Determine how many dots to show (actual count up to maxDots)
	dotsToShow := total
	if dotsToShow > maxDots {
		dotsToShow = maxDots
	}

	// Calculate how many dots for passed vs failed
	passedDots := passed
	failedDots := total - passed

	// If we need to scale down to maxDots, do it proportionally
	if total > maxDots {
		passedDots = (passed * maxDots) / total
		failedDots = maxDots - passedDots
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
			fmt.Sscanf(pkg.StatementCoverage, "%f%%", &pct)
			totalStatementCoverage += pct
		} else if pkg.Coverage != "" && pkg.Coverage != "0.0%" {
			// Fallback to legacy coverage
			packagesWithStatementCoverage++
			var pct float64
			fmt.Sscanf(pkg.Coverage, "%f%%", &pct)
			totalStatementCoverage += pct
		}
		
		// Check function coverage
		if pkg.FunctionCoverage != "" && pkg.FunctionCoverage != "0.0%" && pkg.FunctionCoverage != "N/A" {
			packagesWithFunctionCoverage++
			var pct float64
			fmt.Sscanf(pkg.FunctionCoverage, "%f%%", &pct)
			totalFunctionCoverage += pct
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
	return float64(totalSize) / 1024.0
}

// emitAlert outputs a terminal bell if alert is enabled.
func emitAlert(enabled bool) {
	if enabled {
		fmt.Fprint(os.Stderr, "\a")
	}
}

// displayPackageResult generates the display string for a package result.
func (m *TestModel) displayPackageResult(pkg *PackageResult) string {
	var output strings.Builder

	// Package header
	// Display package header - ▶ icon in white, package name in cyan
	output.WriteString(fmt.Sprintf("▶ %s\n", PackageHeaderStyle.Render(pkg.Package)))

	// Check for "No tests"
	// Check for package-level failures (e.g., TestMain failures)
	if pkg.Status == "fail" && len(pkg.Tests) == 0 {
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

	if pkg.Status == "skip" || m.packagesWithNoTests[pkg.Package] || !pkg.HasTests {
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
		case "pass":
			passedCount++
		case "fail":
			failedCount++
		case "skip":
			skippedCount++
		}
	}

	// Display tests in order (including subtests as individual entries)
	
	// Debug: Log test order details
	if debugFile := os.Getenv("GOTCHA_DEBUG_FILE"); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
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
			if shouldShow || m.showFilter == "collapsed" {
				if !firstTestCheck {
					output.WriteString("\n")
					firstTestCheck = true
				}
				break
			}
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
		if shouldShow || m.showFilter == "collapsed" {
			// Display with appropriate indentation
			indent := ""
			if isSubtest {
				indent = "  " // Indent subtests
			}
			m.displayTestAsLine(&output, test, indent)
		}
	}

	// Always display summary line when package has tests
	// This ensures summaries are shown even when individual tests are filtered out
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		// Add spacing before summary only if tests were displayed
		if firstTestCheck {
			output.WriteString("\n")
		}
		
		var summaryLine string
		coverageStr := ""
		
		// Build coverage string with both statement and function coverage
		if pkg.StatementCoverage != "" && pkg.StatementCoverage != "0.0%" {
			if pkg.FunctionCoverage != "" && pkg.FunctionCoverage != "N/A" {
				coverageStr = fmt.Sprintf(" (statements: %s, functions: %s)", 
					pkg.StatementCoverage, pkg.FunctionCoverage)
			} else {
				// Only statement coverage available from standard Go test output
				coverageStr = fmt.Sprintf(" (%s statement coverage)", pkg.StatementCoverage)
			}
		} else if pkg.Coverage != "" {
			// Fallback to legacy coverage if new fields aren't set
			coverageStr = fmt.Sprintf(" (%s statement coverage)", pkg.Coverage)
		}

		if failedCount > 0 {
			// Show failure summary
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s",
				FailStyle.Render(CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		} else if passedCount > 0 {
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s",
				PassStyle.Render(CheckPass),
				passedCount,
				coverageStr)
		} else if skippedCount > 0 {
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

// displayTest adds a test's display output to the builder.
// This function has been refactored from 133 lines with complexity 102
// to a simple delegation to TestFormatter, reducing complexity to ~5.
func (m *TestModel) displayTest(output *strings.Builder, test *TestResult) {
	formatter := NewTestFormatter(m)
	formatter.FormatTest(output, test)
}

// displayTestAsLine displays a test as a simple one-line entry.
func (m *TestModel) displayTestAsLine(output *strings.Builder, test *TestResult, indent string) {
	// Skip running tests
	if test.Status != "pass" && test.Status != "fail" && test.Status != "skip" {
		return
	}
	
	// Get the status icon
	var icon string
	switch test.Status {
	case "pass":
		icon = PassStyle.Render(CheckPass)
	case "fail":
		icon = FailStyle.Render(CheckFail)
	case "skip":
		icon = SkipStyle.Render(CheckSkip)
	}
	
	// Format the line
	fmt.Fprintf(output, "  %s%s %s", indent, icon, TestNameStyle.Render(test.Name))
	
	// Add duration
	if test.Elapsed > 0 {
		duration := fmt.Sprintf("(%.2fs)", test.Elapsed)
		fmt.Fprintf(output, " %s", DurationStyle.Render(duration))
	}
	
	// Add skip reason if applicable
	if test.Status == "skip" && test.SkipReason != "" {
		reason := fmt.Sprintf("- %s", test.SkipReason)
		fmt.Fprintf(output, " %s", DurationStyle.Render(reason))
	}
	
	output.WriteString("\n")
	
	// Show output for failed tests if not collapsed
	if test.Status == "fail" && m.showFilter != "collapsed" && len(test.Output) > 0 {
		output.WriteString("\n")
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
		output.WriteString("\n")
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
	case "pass":
		styledIcon = PassStyle.Render(CheckPass)
	case "fail":
		styledIcon = FailStyle.Render(CheckFail)
	case "skip":
		styledIcon = SkipStyle.Render(CheckSkip)
	default:
		return // Don't display running tests
	}

	// Display the test
	fmt.Fprintf(output, "  %s %s", styledIcon, TestNameStyle.Render(test.Name))

	// Add duration if available
	if test.Elapsed > 0 {
		fmt.Fprintf(output, " %s", DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Add skip reason if available
	if test.Status == "skip" && test.SkipReason != "" {
		fmt.Fprintf(output, " %s", DurationStyle.Render(fmt.Sprintf("- %s", test.SkipReason)))
	}

	// Add subtest progress indicator if it has subtests
	if hasSubtests && m.subtestStats[test.Name] != nil {
		stats := m.subtestStats[test.Name]
		totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)

		if totalSubtests > 0 {
			miniProgress := m.generateSubtestProgress(len(stats.passed), totalSubtests)
			percentage := (len(stats.passed) * 100) / totalSubtests
			fmt.Fprintf(output, " %s %d%% passed", miniProgress, percentage)
		}
	}

	output.WriteString("\n")

	// Show test output for failed tests if not in collapsed mode
	if test.Status == "fail" && m.showFilter != "collapsed" && len(test.Output) > 0 {
		output.WriteString("\n")
		if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, line := range test.Output {
				// Replace literal \t with actual tabs and \n with newlines
				formatted := strings.ReplaceAll(line, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				output.WriteString("    " + formatted)
			}
		} else {
			// Default: show output as-is
			for _, line := range test.Output {
				output.WriteString("    " + line)
			}
		}
		output.WriteString("\n")
	}

	// Show detailed subtest results for failed parent tests
	if test.Status == "fail" && hasSubtests && m.showFilter != "collapsed" {
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
									formatted = strings.ReplaceAll(formatted, `\n`, "\n")
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
