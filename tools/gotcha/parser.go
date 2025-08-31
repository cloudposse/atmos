package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// parseTestJSON parses go test -json output and returns a TestSummary.
func parseTestJSON(input io.Reader, coverProfile string, excludeMocks bool) (*TestSummary, error) {
	tests := make(map[string]TestResult)
	var coverage string
	var coverageData *CoverageData

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		testCoverage := processLine(line, tests)
		if testCoverage != "" {
			coverage = testCoverage
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	// Parse coverage profile if provided.
	if coverProfile != "" {
		var err error
		coverageData, err = parseCoverageProfile(coverProfile, excludeMocks)
		if err != nil {
			return nil, err
		}
	}

	// Categorize and sort results.
	failed, skipped, passed := categorizeResults(tests)
	sortResults(&failed, &skipped, &passed)

	return &TestSummary{
		Failed:       failed,
		Skipped:      skipped,
		Passed:       passed,
		Coverage:     coverage,
		CoverageData: coverageData,
	}, nil
}

// processLine processes a single line of JSON output.
func processLine(line string, tests map[string]TestResult) string {
	// Try to parse as JSON.
	var event TestEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return "" // Skip non-JSON lines.
	}

	// Extract coverage info from output.
	if event.Action == "output" && strings.Contains(event.Output, "coverage:") {
		if coverage := extractCoverage(event.Output); coverage != "" {
			return coverage
		}
	}

	// Record test results.
	if event.Test != "" && contains([]string{"pass", "fail", "skip"}, event.Action) {
		recordTestResult(&event, tests)
	}

	return ""
}

// extractCoverage extracts coverage percentage from output line.
func extractCoverage(output string) string {
	// Look for "coverage: XX.X% of statements".
	re := regexp.MustCompile(`coverage:\s+(\d+(?:\.\d+)?)%`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1] + "%"
	}
	return ""
}

// recordTestResult records a test result in the tests map.
func recordTestResult(event *TestEvent, tests map[string]TestResult) {
	key := event.Package + "." + event.Test
	tests[key] = TestResult{
		Package:  event.Package,
		Test:     event.Test,
		Status:   event.Action,
		Duration: event.Elapsed,
	}
}

// categorizeResults separates tests by status.
func categorizeResults(tests map[string]TestResult) ([]TestResult, []TestResult, []TestResult) {
	var failed, skipped, passed []TestResult

	for _, test := range tests {
		switch test.Status {
		case "fail":
			failed = append(failed, test)
		case "skip":
			skipped = append(skipped, test)
		case "pass":
			passed = append(passed, test)
		}
	}

	return failed, skipped, passed
}

// sortResults sorts test slices by package and test name.
func sortResults(failed, skipped, passed *[]TestResult) {
	sortFunc := func(i, j int, slice []TestResult) bool {
		if slice[i].Package != slice[j].Package {
			return slice[i].Package < slice[j].Package
		}
		return slice[i].Test < slice[j].Test
	}

	sort.Slice(*failed, func(i, j int) bool { return sortFunc(i, j, *failed) })
	sort.Slice(*skipped, func(i, j int) bool { return sortFunc(i, j, *skipped) })
	sort.Slice(*passed, func(i, j int) bool { return sortFunc(i, j, *passed) })
}

// contains checks if a string slice contains a value.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
