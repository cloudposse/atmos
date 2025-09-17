package stream

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
)

// displayPackageResult outputs the buffered results for a completed package.
//
//nolint:nestif,gocognit // Package result display requires complex conditional logic:
// - Build failure detection and special error formatting
// - No-test-files package handling
// - Coverage information display (statement and function coverage)
// - Test result aggregation and summary generation
// - Different display modes based on package status and content
// This complexity ensures proper package-level reporting for various scenarios.
func (p *StreamProcessor) displayPackageResult(pkg *PackageResult) {
	// Debug: Log package display start
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "\n[DISPLAY-PKG] Starting display for package: %s\n", pkg.Package)
			fmt.Fprintf(f, "  Status: %s, HasTests: %v, TestCount: %d\n",
				pkg.Status, pkg.HasTests, len(pkg.Tests))
			f.Close()
		}
	}

	// Display package header - ▶ icon in white, package name in cyan
	p.writer.PrintUI("\n▶ %s\n",
		tui.PackageHeaderStyle.Render(pkg.Package))

	// Note: We don't call Sync() on stderr because:
	// 1. stderr is typically unbuffered or line-buffered already
	// 2. Sync() is meant for regular files, not console streams
	// 3. Calling Sync() on pipes (like in tests) can cause hangs

	// Check for package-level failures (e.g., build failures, TestMain failures)
	if pkg.Status == TestStatusFail && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely build failure or TestMain failure)
		p.writer.PrintUI("  %s Package failed (build error or initialization failure)\n", tui.FailStyle.Render(tui.CheckFail))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					p.writer.PrintUI("    %s", line)
				}
			}
		}
		return
	}

	// Check if package has no tests
	if !pkg.HasTests {
		// Show more specific message if a filter is applied
		if p.testFilter != "" {
			p.writer.PrintUI("  %s\n", tui.DurationStyle.Render("No tests matching filter"))
		} else {
			p.writer.PrintUI("  %s\n", tui.DurationStyle.Render("No tests"))
		}
		return
	}

	// Count test results for this package (including subtests)
	var passedCount, failedCount, skippedCount int
	for _, test := range pkg.Tests {
		// Count the parent test
		switch test.Status {
		case constants.PassStatus:
			passedCount++
		case TestStatusFail:
			failedCount++
		case TestStatusSkip:
			skippedCount++
		}

		// Count all subtests
		for _, subtest := range test.Subtests {
			switch subtest.Status {
			case constants.PassStatus:
				passedCount++
			case TestStatusFail:
				failedCount++
			case TestStatusSkip:
				skippedCount++
			}
		}
	}

	// Display tests based on show filter
	// Track if any tests were actually displayed
	testsDisplayed := false

	// Debug: Log all tests in TestOrder
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "\n[DISPLAY-DEBUG] Package %s TestOrder:\n", pkg.Package)
			for i, name := range pkg.TestOrder {
				if test := pkg.Tests[name]; test != nil {
					fmt.Fprintf(f, "  [%d] %s: parent=%s, status=%s, subtests=%d\n",
						i, name, test.Parent, test.Status, len(test.Subtests))
				}
			}
			f.Close()
		}
	}

	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]

		// Skip subtests here - they'll be displayed under their parent
		if test.Parent != "" {
			continue
		}

		// For tests without subtests, display normally
		if len(test.Subtests) == 0 {
			if p.shouldShowTestStatus(test.Status) {
				testsDisplayed = true
				p.displayTestLine(test, "")
			}
		} else {
			// For tests with subtests:
			// 1. Display the parent test with mini indicators
			// Debug: Log parent test info
			if debugFile := config.GetDebugFile(); debugFile != "" {
				if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
					fmt.Fprintf(f, "[DISPLAY-DEBUG] Parent test %s: status=%s, subtests=%d, shouldShow=%v\n",
						testName, test.Status, len(test.Subtests), p.shouldShowTestStatus(test.Status))
					f.Close()
				}
			}
			if p.shouldShowTestStatus(test.Status) || test.Status == TestStatusFail {
				testsDisplayed = true
				p.displayTest(test, "")
			}
		}
	}

	// Always display summary line when package has tests
	// This ensures summaries are shown even when individual tests are filtered out
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		// Add spacing before summary only if tests were displayed
		if testsDisplayed {
			p.writer.PrintUI("\n")
		}

		var summaryLine string
		coverageStr := ""
		if pkg.Coverage != "" {
			coverageStr = fmt.Sprintf(" (%s coverage)", pkg.Coverage)
		}

		switch {
		case failedCount > 0:
			// Show failure summary
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s\n",
				tui.FailStyle.Render(tui.CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		case passedCount > 0:
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s\n",
				tui.PassStyle.Render(tui.CheckPass),
				passedCount,
				coverageStr)
		case skippedCount > 0:
			// Only skipped tests
			summaryLine = fmt.Sprintf("  %s %d tests skipped%s\n",
				tui.SkipStyle.Render(tui.CheckSkip),
				skippedCount,
				coverageStr)
		}

		if summaryLine != "" {
			p.writer.PrintUI("%s", summaryLine)
		}
	}

	// Output is already flushed automatically due to line buffering on stderr
}

// displayTestLine outputs a test as a simple one-line entry without subtest progress.
func (p *StreamProcessor) displayTestLine(test *TestResult, indent string) {
	// Skip running tests
	if test.Status != TestStatusPass && test.Status != TestStatusFail && test.Status != TestStatusSkip {
		return
	}

	// Determine status icon
	var statusIcon string
	switch test.Status {
	case constants.PassStatus:
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case constants.FailStatus:
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + "  ")
	line.WriteString(statusIcon)
	line.WriteString(constants.SpaceString)
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(constants.SpaceString)
		line.WriteString(tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Add skip reason if present
	if test.Status == TestStatusSkip && test.SkipReason != "" {
		line.WriteString(constants.SpaceString)
		line.WriteString(tui.FaintStyle.Render("— " + test.SkipReason))
	}

	p.writer.PrintUI("%s\n", line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == TestStatusFail && len(test.Output) > 0 && p.showFilter != "none" {
		if p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, constants.NewlineString)
				p.writer.PrintUI("%s", indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				p.writer.PrintUI("%s", indent+"    "+outputLine)
			}
		}
		p.writer.PrintUI("\n") // Add blank line after output
	}
}

// displayTest outputs a single test result with proper formatting.
func (p *StreamProcessor) displayTest(test *TestResult, indent string) {
	// Check if test has failed subtests (for --show=failed filter)
	hasFailedSubtests := false
	if p.showFilter == "failed" && len(test.Subtests) > 0 {
		for _, subtest := range test.Subtests {
			if subtest.Status == TestStatusFail {
				hasFailedSubtests = true
				break
			}
		}
	}

	// Check if we should display this test based on filter
	if !p.shouldShowTestStatus(test.Status) && !hasFailedSubtests {
		return
	}

	// Determine status icon
	var statusIcon string
	switch test.Status {
	case constants.PassStatus:
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case constants.FailStatus:
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	default:
		return // Don't display running tests
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + "  ")
	line.WriteString(statusIcon)
	line.WriteString(constants.SpaceString)
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(constants.SpaceString)
		line.WriteString(tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Check if test has subtests
	if len(test.Subtests) > 0 {
		// Calculate subtest statistics
		passed := 0
		failed := 0
		skipped := 0

		for _, subtest := range test.Subtests {
			switch subtest.Status {
			case constants.PassStatus:
				passed++
			case TestStatusFail:
				failed++
			case TestStatusSkip:
				skipped++
			}
		}

		total := passed + failed + skipped
		if total > 0 {
			// Add mini progress indicator
			miniProgress := p.generateSubtestProgress(passed, total)
			percentage := (passed * PercentageMultiplier) / total

			line.WriteString(constants.SpaceString)
			line.WriteString(miniProgress)
			line.WriteString(fmt.Sprintf(" %d%% passed", percentage))
		}
	}

	p.writer.PrintUI("%s\n", line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == TestStatusFail && len(test.Output) > 0 && p.showFilter != "none" {
		if p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, constants.NewlineString)
				p.writer.PrintUI("%s", indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				p.writer.PrintUI("%s", indent+"    "+outputLine)
			}
		}
	}

	// Display subtests if test failed or show filter is "all"
	if len(test.Subtests) > 0 && (test.Status == TestStatusFail || p.showFilter == "all") {
		// Display subtest summary for failed tests
		if test.Status == TestStatusFail {
			passed := []*TestResult{}
			failed := []*TestResult{}
			skipped := []*TestResult{}

			for _, subtestName := range test.SubtestOrder {
				subtest := test.Subtests[subtestName]
				switch subtest.Status {
				case constants.PassStatus:
					passed = append(passed, subtest)
				case TestStatusFail:
					failed = append(failed, subtest)
				case TestStatusSkip:
					skipped = append(skipped, subtest)
				}
			}

			total := len(passed) + len(failed) + len(skipped)
			if total > 0 {
				p.writer.PrintUI("\n%s    Subtest Summary: %d passed, %d failed of %d total\n",
					indent, len(passed), len(failed), total)

				// Show passed subtests
				if len(passed) > 0 {
					p.writer.PrintUI("\n%s    %s Passed (%d):\n",
						indent, tui.PassStyle.Render("✔"), len(passed))
					for _, subtest := range passed {
						p.writer.PrintUI("%s      • %s\n", indent, subtest.Name)
					}
				}

				// Show failed subtests
				if len(failed) > 0 {
					p.writer.PrintUI("\n%s    %s Failed (%d):\n",
						indent, tui.FailStyle.Render("✘"), len(failed))
					for _, subtest := range failed {
						p.writer.PrintUI("%s      • %s\n", indent, subtest.Name)
						// Show subtest output if verbosity level is with-output or verbose
						if (p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose") && len(subtest.Output) > 0 {
							for _, outputLine := range subtest.Output {
								formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
								formatted = strings.ReplaceAll(formatted, `\n`, constants.NewlineString)
								p.writer.PrintUI("%s", indent+"        "+formatted)
							}
						}
					}
				}

				// Show skipped subtests
				if len(skipped) > 0 {
					p.writer.PrintUI("\n%s    %s Skipped (%d):\n",
						indent, tui.SkipStyle.Render("⊘"), len(skipped))
					for _, subtest := range skipped {
						p.writer.PrintUI("%s      • %s\n", indent, subtest.Name)
					}
				}
			}
		}
		// For "all" filter, subtests are already shown in mini progress
		// Don't display them again unless specifically requested
	}
}
