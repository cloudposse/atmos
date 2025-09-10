package stream

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

// StreamReporter implements TestReporter for direct stream output to stderr.
// This is the traditional non-TUI display mode that works perfectly.
type StreamReporter struct {
	showFilter     string
	testFilter     string
	verbosityLevel string
	startTime      time.Time
	
	// Track packages we've already displayed to avoid duplicates
	displayedPackages map[string]bool
}

// NewStreamReporter creates a new StreamReporter with the given configuration.
func NewStreamReporter(showFilter, testFilter, verbosityLevel string) *StreamReporter {
	return &StreamReporter{
		showFilter:        showFilter,
		testFilter:        testFilter,
		verbosityLevel:    verbosityLevel,
		startTime:         time.Now(),
		displayedPackages: make(map[string]bool),
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
	fmt.Fprintf(os.Stderr, "\n▶ %s\n",
		tui.PackageHeaderStyle.Render(pkg.Package))

	// Flush output immediately in CI environments to prevent buffering
	if config.IsCI() {
		os.Stderr.Sync()
	}

	// Check for package-level failures (e.g., TestMain failures)
	if pkg.Status == "fail" && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely TestMain failure)
		fmt.Fprintf(os.Stderr, "  %s Package failed to run tests\n", tui.FailStyle.Render(tui.CheckFail))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					fmt.Fprintf(os.Stderr, "    %s", line)
				}
			}
		}
		return
	}

	// Check if package has no tests
	if !pkg.HasTests {
		// Show more specific message if a filter is applied
		if r.testFilter != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", tui.DurationStyle.Render("No tests matching filter"))
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", tui.DurationStyle.Render("No tests"))
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

	// Display tests based on show filter
	// Track if any tests were actually displayed
	testsDisplayed := false
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		
		// For tests without subtests, display normally
		if len(test.Subtests) == 0 {
			if r.shouldShowTestStatus(test.Status) {
				testsDisplayed = true
				r.displayTestLine(test, "")
			}
		} else {
			// For tests with subtests, display each subtest individually
			for _, subtestName := range test.SubtestOrder {
				subtest := test.Subtests[subtestName]
				if r.shouldShowTestStatus(subtest.Status) {
					testsDisplayed = true
					r.displayTestLine(subtest, "  ")
				}
			}
		}
	}

	// Always display summary line when package has tests
	// This ensures summaries are shown even when individual tests are filtered out
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		// Add spacing before summary only if tests were displayed
		if testsDisplayed {
			fmt.Fprintf(os.Stderr, "\n")
		}

		var summaryLine string
		coverageStr := ""
		if pkg.Coverage != "" {
			coverageStr = fmt.Sprintf(" (%s coverage)", pkg.Coverage)
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
			fmt.Fprintf(os.Stderr, "%s", summaryLine)
		}
	}

	// Flush output after displaying package results
	if config.IsCI() {
		os.Stderr.Sync()
	}
}

// displayTestLine outputs a test as a simple one-line entry without subtest progress.
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

	fmt.Fprintln(os.Stderr, line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == "fail" && len(test.Output) > 0 && r.showFilter != "none" {
		if r.verbosityLevel == "with-output" || r.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				fmt.Fprint(os.Stderr, indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				fmt.Fprint(os.Stderr, indent+"    "+outputLine)
			}
		}
		fmt.Fprintln(os.Stderr, "") // Add blank line after output
	}
}

// shouldShowTestStatus determines if a test with the given status should be displayed.
func (r *StreamReporter) shouldShowTestStatus(status string) bool {
	switch r.showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
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

// Finalize is called at the end of all test execution and returns any final output.
func (r *StreamReporter) Finalize(passed, failed, skipped int, elapsed time.Duration) string {
	total := passed + failed + skipped
	if total == 0 {
		return ""
	}

	var output strings.Builder
	
	output.WriteString("\n\n")
	output.WriteString(fmt.Sprintf("%s\n", tui.StatsHeaderStyle.Render("Test Results:")))
	output.WriteString(fmt.Sprintf("  %s Passed:  %d\n", tui.PassStyle.Render(tui.CheckPass), passed))
	output.WriteString(fmt.Sprintf("  %s Failed:  %d\n", tui.FailStyle.Render(tui.CheckFail), failed))
	output.WriteString(fmt.Sprintf("  %s Skipped: %d\n", tui.SkipStyle.Render(tui.CheckSkip), skipped))
	output.WriteString(fmt.Sprintf("  Total:     %d\n", total))
	
	// TODO: Add average coverage calculation when we have package results
	
	output.WriteString("\n")
	output.WriteString(fmt.Sprintf("%s Tests completed in %.2fs\n", tui.DurationStyle.Render("ℹ"), elapsed.Seconds()))
	
	// Write to stderr and return
	fmt.Fprint(os.Stderr, output.String())
	
	// Ensure output is flushed
	if config.IsCI() {
		os.Stderr.Sync()
	}
	
	return ""
}