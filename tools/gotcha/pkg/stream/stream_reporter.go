package stream

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
)

// StreamReporter implements TestReporter for direct stream output.
// This is the traditional non-TUI display mode that works perfectly.
type StreamReporter struct {
	writer         *output.Writer
	showFilter     string
	testFilter     string
	verbosityLevel string
	startTime      time.Time

	// Track packages we've already displayed to avoid duplicates
	displayedPackages map[string]bool

	// Track package coverage for total calculation
	packageCoverages          []float64
	packageStatementCoverages []float64
	packageFunctionCoverages  []float64
}

// NewStreamReporter creates a new StreamReporter with the given configuration.
func NewStreamReporter(writer *output.Writer, showFilter, testFilter, verbosityLevel string) *StreamReporter {
	if writer == nil {
		writer = output.New()
	}
	return &StreamReporter{
		writer:                    writer,
		showFilter:                showFilter,
		testFilter:                testFilter,
		verbosityLevel:            verbosityLevel,
		startTime:                 time.Now(),
		displayedPackages:         make(map[string]bool),
		packageCoverages:          make([]float64, 0),
		packageStatementCoverages: make([]float64, 0),
		packageFunctionCoverages:  make([]float64, 0),
	}
}

// OnPackageStart is called when a package starts testing.
func (r *StreamReporter) OnPackageStart(pkg *PackageResult) {
	// Stream reporter doesn't display anything on package start
	// It waits for package completion to display all results at once
}

// OnPackageComplete is called when a package finishes testing.
func (r *StreamReporter) OnPackageComplete(pkg *PackageResult) {
	// Avoid displaying the same package multiple times
	if r.displayedPackages[pkg.Package] {
		return
	}
	r.displayedPackages[pkg.Package] = true

	// Display package header - ▶ icon in white, package name in cyan
	r.writer.PrintUI("▶ %s\n",
		tui.PackageHeaderStyle.Render(pkg.Package))

	// Check for package-level failures (e.g., build failures, TestMain failures)
	if pkg.Status == "fail" && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely build failure or TestMain failure)
		r.writer.PrintUI("  %s Package failed (build error or initialization failure)\n", tui.FailStyle.Render(tui.CheckFail))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					r.writer.PrintUI("    %s", line)
				}
			}
		}
		return
	}

	// Check if package has no tests
	if !pkg.HasTests {
		// Show more specific message if a filter is applied
		if r.testFilter != "" {
			r.writer.PrintUI("  %s\n", tui.DurationStyle.Render("No tests matching filter"))
		} else {
			r.writer.PrintUI("  %s\n", tui.DurationStyle.Render("No tests"))
		}
		return
	}

	// Count test results for this package (including subtests)
	var passedCount, failedCount, skippedCount int
	for _, test := range pkg.Tests {
		// Count the parent test
		switch test.Status {
		case "pass":
			passedCount++
		case "fail":
			failedCount++
		case "skip":
			skippedCount++
		}

		// Count all subtests
		for _, subtest := range test.Subtests {
			switch subtest.Status {
			case "pass":
				passedCount++
			case "fail":
				failedCount++
			case "skip":
				skippedCount++
			}
		}
	}

	// Add blank line before tests section
	r.writer.PrintUI("\n")

	// Display tests based on show filter
	testsDisplayed := false
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]

		// Skip subtests here - they'll be displayed under their parent
		if test.Parent != "" {
			continue
		}

		// For tests without subtests, display normally
		if len(test.Subtests) == 0 {
			if r.shouldShowTestStatus(test.Status) {
				testsDisplayed = true
				r.displayTestLine(test, "")
			}
		} else {
			// For tests with subtests:
			// 1. Display the parent test with mini indicators
			if r.shouldShowTestStatus(test.Status) || test.Status == "fail" {
				testsDisplayed = true
				r.displayTest(test, "")
			}
		}
	}

	// Always display summary line when package has tests
	// This ensures summaries are shown even when individual tests are filtered out
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		// Add spacing before summary only if individual tests were displayed
		if testsDisplayed {
			r.writer.PrintUI("\n")
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
			// Parse and store statement coverage for total calculation
			if coverageValue := parseCoverageValue(pkg.StatementCoverage); coverageValue >= 0 {
				r.packageStatementCoverages = append(r.packageStatementCoverages, coverageValue)
				r.packageCoverages = append(r.packageCoverages, coverageValue) // Keep for backward compat
			}
			// Parse and store function coverage if available
			if pkg.FunctionCoverage != "" && pkg.FunctionCoverage != "N/A" {
				if funcCoverage := parseCoverageValue(pkg.FunctionCoverage); funcCoverage >= 0 {
					r.packageFunctionCoverages = append(r.packageFunctionCoverages, funcCoverage)
				}
			}
		} else if pkg.Coverage != "" {
			// Fallback to legacy coverage if new fields aren't set
			coverageStr = fmt.Sprintf(" (%s coverage)", pkg.Coverage)
			if coverageValue := parseCoverageValue(pkg.Coverage); coverageValue >= 0 {
				r.packageCoverages = append(r.packageCoverages, coverageValue)
				r.packageStatementCoverages = append(r.packageStatementCoverages, coverageValue)
			}
		}

		if failedCount > 0 {
			// Show failure summary
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s\n",
				tui.FailStyle.Render(tui.CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		} else if passedCount > 0 {
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s\n",
				tui.PassStyle.Render(tui.CheckPass),
				passedCount,
				coverageStr)
		} else if skippedCount > 0 {
			// Only skipped tests
			summaryLine = fmt.Sprintf("  %s %d tests skipped%s\n",
				tui.SkipStyle.Render(tui.CheckSkip),
				skippedCount,
				coverageStr)
		}

		if summaryLine != "" {
			r.writer.PrintUI("%s", summaryLine)
		}
	}

	// Output is already flushed automatically due to line buffering on stderr
}

// displayTestLine outputs a test as a simple one-line entry without subtest progress.
// DisplayTest outputs a test result with mini indicators for subtests.
func (r *StreamReporter) displayTest(test *TestResult, indent string) {
	// Skip running tests
	if test.Status != "pass" && test.Status != "fail" && test.Status != "skip" {
		return
	}

	// Determine status icon
	var statusIcon string
	switch test.Status {
	case "pass":
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case "fail":
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + "  ")
	line.WriteString(statusIcon)
	line.WriteString(" ")
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(" ")
		line.WriteString(tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Add mini progress indicator for tests with subtests
	if len(test.Subtests) > 0 {
		// Calculate subtest statistics
		passed := 0
		failed := 0
		skipped := 0

		for _, subtest := range test.Subtests {
			switch subtest.Status {
			case "pass":
				passed++
			case "fail":
				failed++
			case "skip":
				skipped++
			}
		}

		total := passed + failed + skipped
		if total > 0 {
			// Add mini progress indicator
			miniProgress := r.generateSubtestProgress(passed, total)
			percentage := (passed * 100) / total

			line.WriteString(" ")
			line.WriteString(miniProgress)
			line.WriteString(fmt.Sprintf(" %d%% passed", percentage))
		}
	}

	r.writer.PrintUI("%s\n", line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == "fail" && len(test.Output) > 0 && r.showFilter != "none" {
		if r.verbosityLevel == "with-output" || r.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				r.writer.PrintUI("%s", indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				r.writer.PrintUI("%s", indent+"    "+outputLine)
			}
		}
		r.writer.PrintUI("\n") // Add blank line after output
	}

	// Display subtests if test failed or show filter is "all"
	if len(test.Subtests) > 0 && (test.Status == "fail" || r.showFilter == "all") {
		for _, subtestName := range test.SubtestOrder {
			subtest := test.Subtests[subtestName]
			if r.shouldShowTestStatus(subtest.Status) {
				r.displayTestLine(subtest, indent+"  ")
			}
		}
	}
}

func (r *StreamReporter) displayTestLine(test *TestResult, indent string) {
	// Skip running tests
	if test.Status != "pass" && test.Status != "fail" && test.Status != "skip" {
		return
	}

	// Determine status icon
	var statusIcon string
	switch test.Status {
	case "pass":
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case "fail":
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + "  ")
	line.WriteString(statusIcon)
	line.WriteString(" ")
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(" ")
		line.WriteString(tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Add skip reason if present
	if test.Status == "skip" && test.SkipReason != "" {
		line.WriteString(" ")
		line.WriteString(tui.FaintStyle.Render("— " + test.SkipReason))
	}

	r.writer.PrintUI("%s\n", line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == "fail" && len(test.Output) > 0 && r.showFilter != "none" {
		if r.verbosityLevel == "with-output" || r.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				r.writer.PrintUI("%s", indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				r.writer.PrintUI("%s", indent+"    "+outputLine)
			}
		}
		r.writer.PrintUI("\n") // Add blank line after output
	}
}

// shouldShowTestStatus determines if a test with the given status should be displayed.
func (r *StreamReporter) shouldShowTestStatus(status string) bool {
	switch r.showFilter {
	case "all":
		return true
	case "failed":
		// Show both failed and skipped tests when filter is "failed"
		return status == "fail" || status == "skip"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	case "collapsed", "none":
		return false
	default:
		return true
	}
}

// OnTestStart is called when an individual test starts.
func (r *StreamReporter) OnTestStart(pkg *PackageResult, test *TestResult) {
	// Stream reporter doesn't display anything during test execution
	// It waits for package completion to display all results at once
}

// OnTestComplete is called when an individual test completes.
func (r *StreamReporter) OnTestComplete(pkg *PackageResult, test *TestResult) {
	// Stream reporter doesn't display anything during test execution
	// It waits for package completion to display all results at once
}

// UpdateProgress updates the overall progress of test execution.
func (r *StreamReporter) UpdateProgress(completed, total int, elapsed time.Duration) {
	// Stream reporter doesn't display progress during execution
}

// SetEstimatedTotal sets the estimated total number of tests.
func (r *StreamReporter) SetEstimatedTotal(total int) {
	// Stream reporter doesn't use estimates
}

// parseCoverageValue extracts the numeric coverage percentage from a coverage string.
// Returns -1 if the coverage value cannot be parsed.
func parseCoverageValue(coverage string) float64 {
	// Remove "% of statements" or just "%" suffix
	coverage = strings.TrimSuffix(coverage, " of statements")
	coverage = strings.TrimSuffix(coverage, "%")
	coverage = strings.TrimSpace(coverage)

	// Parse the numeric value
	value, err := strconv.ParseFloat(coverage, 64)
	if err != nil {
		return -1
	}
	return value
}

// Finalize is called at the end of all test execution and returns any final output.
func (r *StreamReporter) Finalize(passed, failed, skipped int, elapsed time.Duration) string {
	total := passed + failed + skipped
	if total == 0 {
		return ""
	}

	var output strings.Builder

	output.WriteString("\n\n")
	output.WriteString(fmt.Sprintf("%s\n", tui.StatsHeaderStyle.Render("Test Results:")))
	// Right-align numbers for better readability
	output.WriteString(fmt.Sprintf("  %s Passed:  %5d\n", tui.PassStyle.Render(tui.CheckPass), passed))
	output.WriteString(fmt.Sprintf("  %s Failed:  %5d\n", tui.FailStyle.Render(tui.CheckFail), failed))
	output.WriteString(fmt.Sprintf("  %s Skipped: %5d\n", tui.SkipStyle.Render(tui.CheckSkip), skipped))
	output.WriteString(fmt.Sprintf("  Total:     %5d\n", total))

	// Add coverage calculations for both statement and function coverage
	if len(r.packageStatementCoverages) > 0 {
		// Calculate statement coverage average
		totalStatementCoverage := 0.0
		for _, cov := range r.packageStatementCoverages {
			totalStatementCoverage += cov
		}
		avgStatementCoverage := totalStatementCoverage / float64(len(r.packageStatementCoverages))

		// Calculate function coverage average if available
		if len(r.packageFunctionCoverages) > 0 {
			totalFunctionCoverage := 0.0
			for _, cov := range r.packageFunctionCoverages {
				totalFunctionCoverage += cov
			}
			avgFunctionCoverage := totalFunctionCoverage / float64(len(r.packageFunctionCoverages))

			// Show both types
			output.WriteString(fmt.Sprintf("  Statement Coverage: %5.1f%%\n", avgStatementCoverage))
			output.WriteString(fmt.Sprintf("  Function Coverage:  %5.1f%%\n", avgFunctionCoverage))
		} else {
			// Only statement coverage available (standard Go test output)
			// Display as "Statement Coverage" to be explicit about what we're showing
			output.WriteString(fmt.Sprintf("  Statement Coverage: %5.1f%%\n", avgStatementCoverage))
		}
	} else if len(r.packageCoverages) > 0 {
		// Fallback to legacy coverage calculation
		// This is statement coverage from standard Go test output
		totalCoverage := 0.0
		for _, cov := range r.packageCoverages {
			totalCoverage += cov
		}
		avgCoverage := totalCoverage / float64(len(r.packageCoverages))
		output.WriteString(fmt.Sprintf("  Statement Coverage: %5.1f%%\n", avgCoverage))
	}

	output.WriteString("\n")
	output.WriteString(fmt.Sprintf("%s Tests completed in %.2fs\n", tui.DurationStyle.Render("ℹ"), elapsed.Seconds()))

	// Add exit status information
	if failed > 0 {
		output.WriteString(fmt.Sprintf("\n%s %d test%s failed\n", 
			tui.FailStyle.Render("✗"), 
			failed,
			pluralize(failed)))
	} else if total == 0 {
		output.WriteString(fmt.Sprintf("\n%s No tests found\n", tui.SkipStyle.Render("⚠")))
	} else {
		output.WriteString(fmt.Sprintf("\n%s All tests passed\n", tui.PassStyle.Render("✓")))
	}

	// Write to stderr and return
	r.writer.PrintUI("%s", output.String())

	// Output is already flushed automatically due to line buffering on stderr

	return output.String()
}

// pluralize returns "s" if count is not 1, otherwise empty string.
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (r *StreamReporter) generateSubtestProgress(passed, total int) string {
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
		indicator.WriteString(tui.PassStyle.Render("●"))
	}

	// Add red dots for failed tests
	for i := 0; i < failedDots; i++ {
		indicator.WriteString(tui.FailStyle.Render("●"))
	}

	return indicator.String()
}
