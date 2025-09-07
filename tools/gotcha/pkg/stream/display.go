package stream

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
)

// displayPackageResult outputs the buffered results for a completed package.
func (p *StreamProcessor) displayPackageResult(pkg *PackageResult) {
	// Display package header - ▶ icon in white, package name in cyan
	fmt.Fprintf(os.Stderr, "\n▶ %s\n",
		tui.PackageHeaderStyle.Render(pkg.Package))

	// Flush output immediately in CI environments to prevent buffering
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
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
		if p.testFilter != "" {
			fmt.Fprintf(os.Stderr, " %s\n", tui.DurationStyle.Render("No tests matching filter"))
		} else {
			fmt.Fprintf(os.Stderr, " %s\n", tui.DurationStyle.Render("No tests"))
		}
		return
	}

	// Count test results for this package
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

	// Display tests based on show filter
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		p.displayTest(test, "")
	}

	// Display summary line with test counts and coverage
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
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
			fmt.Fprintf(os.Stderr, "\n%s", summaryLine)
		}
	}

	// Flush output after displaying package results
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		os.Stderr.Sync()
	}
}

// displayTest outputs a single test result with proper formatting.
func (p *StreamProcessor) displayTest(test *TestResult, indent string) {
	// Check if test has failed subtests (for --show=failed filter)
	hasFailedSubtests := false
	if p.showFilter == "failed" && len(test.Subtests) > 0 {
		for _, subtest := range test.Subtests {
			if subtest.Status == "fail" {
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
	case "pass":
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case "fail":
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	default:
		return // Don't display running tests
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + " ")
	line.WriteString(statusIcon)
	line.WriteString(" ")
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(" ")
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
			miniProgress := p.generateSubtestProgress(passed, total)
			percentage := (passed * 100) / total

			line.WriteString(" ")
			line.WriteString(miniProgress)
			line.WriteString(fmt.Sprintf(" %d%% passed", percentage))
		}
	}

	fmt.Fprintln(os.Stderr, line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == "fail" && len(test.Output) > 0 && p.showFilter != "none" {
		if p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose" {
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
	}

	// Display subtests if test failed or show filter is "all"
	if len(test.Subtests) > 0 && (test.Status == "fail" || p.showFilter == "all") {
		// Display subtest summary for failed tests
		if test.Status == "fail" {
			passed := []*TestResult{}
			failed := []*TestResult{}
			skipped := []*TestResult{}

			for _, subtestName := range test.SubtestOrder {
				subtest := test.Subtests[subtestName]
				switch subtest.Status {
				case "pass":
					passed = append(passed, subtest)
				case "fail":
					failed = append(failed, subtest)
				case "skip":
					skipped = append(skipped, subtest)
				}
			}

			total := len(passed) + len(failed) + len(skipped)
			if total > 0 {
				fmt.Fprintf(os.Stderr, "\n%s    Subtest Summary: %d passed, %d failed of %d total\n",
					indent, len(passed), len(failed), total)

				// Show passed subtests
				if len(passed) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Passed (%d):\n",
						indent, tui.PassStyle.Render("✔"), len(passed))
					for _, subtest := range passed {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
					}
				}

				// Show failed subtests
				if len(failed) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Failed (%d):\n",
						indent, tui.FailStyle.Render("✘"), len(failed))
					for _, subtest := range failed {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
						// Show subtest output if verbosity level is with-output or verbose
						if (p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose") && len(subtest.Output) > 0 {
							for _, outputLine := range subtest.Output {
								formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
								formatted = strings.ReplaceAll(formatted, `\n`, "\n")
								fmt.Fprint(os.Stderr, indent+"        "+formatted)
							}
						}
					}
				}

				// Show skipped subtests
				if len(skipped) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Skipped (%d):\n",
						indent, tui.SkipStyle.Render("⊘"), len(skipped))
					for _, subtest := range skipped {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
					}
				}
			}
		} else if p.showFilter == "all" {
			// For "all" filter, subtests are already shown in mini progress
			// Don't display them again unless specifically requested
		}
	}
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (p *StreamProcessor) generateSubtestProgress(passed, total int) string {
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