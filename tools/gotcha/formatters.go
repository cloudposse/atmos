package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// writeFailedTests writes the failed tests section.
func writeFailedTests(output io.Writer, failed []TestResult) {
	if len(failed) == 0 {
		return // Hide entire section when no failures
	}

	fmt.Fprintf(output, "### ❌ Failed Tests (%d)\n\n", len(failed))

	fmt.Fprint(output, detailsOpenTag)
	fmt.Fprintf(output, "<summary>Click to see failed tests</summary>\n\n")
	fmt.Fprintf(output, "| Test | Package | Duration |\n")
	fmt.Fprintf(output, "|------|---------|----------|\n")
	for _, test := range failed {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
	}
	fmt.Fprintf(output, "\n**Run locally to reproduce:**\n")
	fmt.Fprintf(output, "```bash\n")
	for _, test := range failed {
		fmt.Fprintf(output, "go test %s -run ^%s$ -v\n", test.Package, test.Test)
	}
	fmt.Fprintf(output, "```"+detailsCloseTag)
}

// writeSkippedTests writes the skipped tests section.
func writeSkippedTests(output io.Writer, skipped []TestResult) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(output, "### ⏭️ Skipped Tests (%d)\n\n", len(skipped))
	fmt.Fprint(output, detailsOpenTag)
	fmt.Fprintf(output, "<summary>Click to see skipped tests</summary>\n\n")
	fmt.Fprintf(output, "| Test | Package |\n")
	fmt.Fprintf(output, "|------|--------|\n")
	for _, test := range skipped {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
	}
	fmt.Fprint(output, detailsCloseTag)
}

// writePassedTests writes the passed tests section with hybrid strategy.
func writePassedTests(output io.Writer, passed []TestResult) {
	if len(passed) == 0 {
		return
	}

	fmt.Fprintf(output, "### ✅ Passed Tests (%d)\n\n", len(passed))

	// For small number of tests, show all in one block.
	if len(passed) < minTestsForSmartDisplay {
		fmt.Fprint(output, detailsOpenTag)
		fmt.Fprintf(output, "<summary>Click to show all passing tests</summary>\n\n")
		totalDuration := calculateTotalDuration(passed)
		writeTestTable(output, passed, true, totalDuration)
		fmt.Fprint(output, detailsCloseTag)
		return
	}

	// For large number of tests, use hybrid strategy.
	changedPackages := getChangedPackages()
	changedTests := filterTestsByPackages(passed, changedPackages)
	slowestTests := getTopSlowestTests(passed, maxSlowestTests)
	packageSummaries := generatePackageSummary(passed)

	testsShown := len(changedTests) + len(slowestTests)
	fmt.Fprintf(output, "Showing %d of %d passed tests.\n\n", testsShown, len(passed))

	// Show tests from changed packages.
	if len(changedTests) > 0 {
		fmt.Fprint(output, detailsOpenTag)
		fmt.Fprintf(output, "<summary>📝 Tests in Changed Packages (%d)</summary>\n\n", len(changedTests))
		totalDuration := calculateTotalDuration(changedTests)
		writeTestTable(output, changedTests, true, totalDuration)
		fmt.Fprint(output, detailsCloseTag)
	}

	// Show slowest tests.
	if len(slowestTests) > 0 {
		fmt.Fprint(output, detailsOpenTag)
		fmt.Fprintf(output, "<summary>⏱️ Slowest Tests (%d)</summary>\n\n", len(slowestTests))
		totalDuration := calculateTotalDuration(passed) // Use all passed tests for total
		writeTestTable(output, slowestTests, true, totalDuration)
		fmt.Fprint(output, detailsCloseTag)
	}

	// Show package summary.
	if len(packageSummaries) > 0 {
		fmt.Fprint(output, detailsOpenTag)
		fmt.Fprintf(output, "<summary>📊 Package Summary</summary>\n\n")

		// Calculate total duration across all packages.
		totalDuration := calculateTotalDuration(passed)

		fmt.Fprintf(output, "| Package | Tests Passed | Avg Duration | Total Duration | %% of Total |\n")
		fmt.Fprintf(output, "|---------|--------------|--------------|----------------|----------|\n")
		for _, summary := range packageSummaries {
			percentage := (summary.TotalDuration / totalDuration) * percentageMultiplier
			fmt.Fprintf(output, "| %s | %d | %.3fs | %.2fs | %.1f%% |\n",
				summary.Package, summary.TestCount, summary.AvgDuration, summary.TotalDuration, percentage)
		}
		fmt.Fprint(output, detailsCloseTag)
	}
}

// writeTestTable writes a table of tests.
func writeTestTable(output io.Writer, tests []TestResult, includeDuration bool, totalDuration float64) {
	if includeDuration {
		if totalDuration > 0 {
			fmt.Fprintf(output, "| Test | Package | Duration | %% of Total |\n")
			fmt.Fprintf(output, "|------|---------|----------|----------|\n")
			for _, test := range tests {
				pkg := shortPackage(test.Package)
				percentage := (test.Duration / totalDuration) * percentageMultiplier
				fmt.Fprintf(output, "| `%s` | %s | %.2fs | %.1f%% |\n", test.Test, pkg, test.Duration, percentage)
			}
		} else {
			fmt.Fprintf(output, "| Test | Package | Duration |\n")
			fmt.Fprintf(output, "|------|---------|----------|\n")
			for _, test := range tests {
				pkg := shortPackage(test.Package)
				fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
			}
		}
	} else {
		fmt.Fprintf(output, "| Test | Package |\n")
		fmt.Fprintf(output, "|------|--------|\n")
		for _, test := range tests {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
		}
	}
	fmt.Fprintf(output, "\n")
}

// calculateTotalDuration calculates the total duration from a list of test results.
func calculateTotalDuration(tests []TestResult) float64 {
	var total float64
	for _, test := range tests {
		total += test.Duration
	}
	return total
}

// getCoverageEmoji returns the appropriate emoji for a coverage percentage.
func getCoverageEmoji(percentage float64) string {
	if percentage >= coverageHighThreshold {
		return "🟢"
	} else if percentage >= coverageMedThreshold {
		return "🟡"
	}
	return "🔴"
}

// calculateFunctionCoverage calculates function coverage statistics.
func calculateFunctionCoverage(functions []CoverageFunction) (covered, total int, percentage float64) {
	total = len(functions)
	for _, fn := range functions {
		if fn.Coverage > 0 {
			covered++
		}
	}
	if total > 0 {
		percentage = (float64(covered) / float64(total)) * percentageMultiplier
	}
	return
}

// writeTestCoverageSection writes the test coverage section with table format.
func writeTestCoverageSection(output io.Writer, coverageData *CoverageData) {
	if coverageData == nil {
		return
	}

	fmt.Fprintf(output, "# Test Coverage\n\n")

	// Build statement coverage details.
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverageData.StatementCoverage, "%"), base10BitSize)
	statementEmoji := getCoverageEmoji(coverageFloat)

	statementDetails := statementEmoji
	if len(coverageData.FilteredFiles) > 0 {
		statementDetails += fmt.Sprintf(" (excluded %d mock files)", len(coverageData.FilteredFiles))
	}

	// Calculate function coverage statistics.
	coveredFunctions, totalFunctions, functionCoveragePercent := calculateFunctionCoverage(coverageData.FunctionCoverage)
	funcEmoji := getCoverageEmoji(functionCoveragePercent)
	functionDetails := fmt.Sprintf("%s %d/%d functions covered", funcEmoji, coveredFunctions, totalFunctions)

	// Write coverage table.
	fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
	fmt.Fprintf(output, "|--------|----------|----------|\n")
	fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n", coverageData.StatementCoverage, statementDetails)
	fmt.Fprintf(output, "| Function Coverage | %.1f%% | %s |\n\n", functionCoveragePercent, functionDetails)

	// Show uncovered functions from changed files only.
	if len(coverageData.FunctionCoverage) > 0 {
		writePRFilteredUncoveredFunctions(output, coverageData.FunctionCoverage)
	}
}

// writePRFilteredUncoveredFunctions writes uncovered functions filtered by PR changes.
func writePRFilteredUncoveredFunctions(output io.Writer, functions []CoverageFunction) {
	changedFiles := getChangedFiles()
	if len(changedFiles) == 0 {
		// No changed files detected, skip this section.
		return
	}

	uncoveredInPR, totalUncovered := getUncoveredFunctionsInPR(functions, changedFiles)

	// Only show if there are uncovered functions in PR files.
	if len(uncoveredInPR) > 0 {
		writeUncoveredFunctionsTable(output, uncoveredInPR, totalUncovered)
	}
}

// getUncoveredFunctionsInPR filters uncovered functions to those in changed files.
func getUncoveredFunctionsInPR(functions []CoverageFunction, changedFiles []string) ([]CoverageFunction, int) {
	// Create set of changed files for faster lookup.
	changedFileSet := make(map[string]bool)
	for _, file := range changedFiles {
		changedFileSet[file] = true
	}

	// Filter functions to only those in changed files.
	var uncoveredInPR []CoverageFunction
	totalInChangedFiles := 0

	for _, fn := range functions {
		// Check if this function's file is in the changed files.
		if changedFileSet[fn.File] {
			totalInChangedFiles++
			if fn.Coverage == 0 {
				uncoveredInPR = append(uncoveredInPR, fn)
			}
		}
	}

	return uncoveredInPR, totalInChangedFiles
}

// writeUncoveredFunctionsTable writes the table of uncovered functions.
func writeUncoveredFunctionsTable(output io.Writer, functions []CoverageFunction, total int) {
	fmt.Fprint(output, detailsOpenTag)
	fmt.Fprintf(output, "<summary>❌ Uncovered Functions in This PR (%d of %d)</summary>\n\n", len(functions), total)
	fmt.Fprintf(output, "| Function | File |\n")
	fmt.Fprintf(output, "|----------|------|\n")
	for _, fn := range functions {
		file := shortPackage(fn.File)
		fmt.Fprintf(output, "| `%s` | %s |\n", fn.Function, file)
	}
	fmt.Fprint(output, detailsCloseTag)
}

// writeLegacyCoverageSection writes coverage in the legacy table format.
func writeLegacyCoverageSection(output io.Writer, coverage string) {
	fmt.Fprintf(output, "# Test Coverage\n\n")
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 40%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 40-79%.
	}

	fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
	fmt.Fprintf(output, "|--------|----------|----------|\n")
	fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n\n", coverage, emoji)
}
