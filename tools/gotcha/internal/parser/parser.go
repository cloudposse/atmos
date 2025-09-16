package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	coveragePkg "github.com/cloudposse/atmos/tools/gotcha/internal/coverage"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// parseTestJSON parses go test -json output and returns a TestSummary.
// ParseTestJSON parses test JSON output and returns a test summary.
func ParseTestJSON(input io.Reader, coverProfile string, excludeMocks bool) (*types.TestSummary, error) {
	tests := make(map[string]types.TestResult)
	skipReasons := make(map[string]string) // Track skip reasons separately
	var coverage string
	var coverageData *types.CoverageData
	var totalElapsedTime float64

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		testCoverage, pkgElapsed := processLineWithElapsedAndSkipReason(line, tests, skipReasons)
		if testCoverage != "" {
			coverage = testCoverage
		}
		// Track the maximum package elapsed time (overall test run time)
		if pkgElapsed > totalElapsedTime {
			totalElapsedTime = pkgElapsed
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	// Parse coverage profile if provided.
	if coverProfile != "" {
		var err error
		coverageData, err = coveragePkg.ParseCoverageProfile(coverProfile, excludeMocks)
		if err != nil {
			return nil, err
		}
	}

	// Categorize and sort results.
	failed, skipped, passed := categorizeResults(tests, skipReasons)
	sortResults(&failed, &skipped, &passed)

	return &types.TestSummary{
		Failed:           failed,
		Skipped:          skipped,
		Passed:           passed,
		Coverage:         coverage,
		CoverageData:     coverageData,
		TotalElapsedTime: totalElapsedTime,
	}, nil
}

// processLineWithElapsedAndSkipReason processes a single line of JSON output and returns coverage and elapsed time.
// It also captures skip reasons from test output.
func processLineWithElapsedAndSkipReason(line string, tests map[string]types.TestResult, skipReasons map[string]string) (string, float64) {
	// Try to parse as JSON.
	var event types.TestEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return "", 0 // Skip non-JSON lines.
	}

	var coverage string
	var elapsed float64

	// Extract coverage info from output.
	if event.Action == "output" && strings.Contains(event.Output, "coverage:") {
		if cov := extractCoverage(event.Output); cov != "" {
			coverage = cov
		}
	}

	// Capture skip reason from output
	if event.Action == "output" && event.Test != "" {
		output := strings.TrimSpace(event.Output)
		key := event.Package + "." + event.Test

		// Two patterns to handle:
		// Pattern 1: Skip reason appears BEFORE --- SKIP line (common with t.Skipf)
		// Pattern 2: Skip reason appears AFTER --- SKIP line (common with t.Skip in subtests)

		if strings.Contains(output, "--- SKIP:") {
			// Mark that we found a skip, reason might come in next output
			if _, exists := skipReasons[key]; !exists {
				skipReasons[key] = "" // Mark as pending
			}
		} else if output != "" && !strings.HasPrefix(output, "===") {
			// Check if this is a skip reason
			// Look for common skip patterns
			if strings.Contains(output, "Skipping") || strings.Contains(output, "Skip") || strings.Contains(output, "skipping") {
				// Extract the reason from the output
				reason := output
				// If it has the pattern "filename.go:linenum: message", extract the message
				if idx := strings.Index(reason, ".go:"); idx > 0 {
					// Find the colon after the line number
					afterFile := reason[idx+4:] // Skip past ".go:"
					if colonIdx := strings.Index(afterFile, ": "); colonIdx > 0 {
						// Extract everything after "filename.go:linenum: "
						reason = strings.TrimSpace(afterFile[colonIdx+2:])
					}
				} else {
					// No file reference, try to extract after first colon
					if idx := strings.Index(reason, ": "); idx > 0 {
						reason = strings.TrimSpace(reason[idx+2:])
					}
				}
				if reason != "" {
					skipReasons[key] = reason
				}
			} else if _, tracking := skipReasons[key]; tracking && skipReasons[key] == "" {
				// This might be the skip reason after --- SKIP line
				// Handle format: "    filename.go:line: reason"
				reason := output
				// Look for pattern: filename.go:linenum:
				if idx := strings.Index(reason, ".go:"); idx > 0 {
					// Find the colon after the line number
					afterFile := reason[idx+4:] // Skip past ".go:"
					if colonIdx := strings.Index(afterFile, ": "); colonIdx > 0 {
						// Extract everything after "filename.go:linenum: "
						reason = strings.TrimSpace(afterFile[colonIdx+2:])
					}
				}
				if reason != "" && reason != output {
					skipReasons[key] = reason
				}
			}
		}
	}

	// Record test results.
	if event.Test != "" && contains([]string{"pass", "fail", "skip"}, event.Action) {
		recordTestResult(&event, tests)
	}

	// Capture package-level elapsed time (overall test duration)
	if event.Test == "" && (event.Action == "pass" || event.Action == "fail") && event.Elapsed > 0 {
		elapsed = event.Elapsed
	}

	return coverage, elapsed
}

// processLineWithElapsed processes a single line of JSON output and returns coverage and elapsed time.
// Kept for backward compatibility.
func processLineWithElapsed(line string, tests map[string]types.TestResult) (string, float64) {
	skipReasons := make(map[string]string)
	return processLineWithElapsedAndSkipReason(line, tests, skipReasons)
}

// processLine processes a single line of JSON output (kept for backward compatibility).
func processLine(line string, tests map[string]types.TestResult) string {
	coverage, _ := processLineWithElapsed(line, tests)
	return coverage
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
func recordTestResult(event *types.TestEvent, tests map[string]types.TestResult) {
	key := event.Package + "." + event.Test
	tests[key] = types.TestResult{
		Package:  event.Package,
		Test:     event.Test,
		Status:   event.Action,
		Duration: event.Elapsed,
	}
}

// categorizeResults separates tests by status and adds skip reasons.
func categorizeResults(tests map[string]types.TestResult, skipReasons map[string]string) ([]types.TestResult, []types.TestResult, []types.TestResult) {
	var failed, skipped, passed []types.TestResult

	for key, test := range tests {
		// Add skip reason if available
		if test.Status == "skip" {
			if reason, exists := skipReasons[key]; exists {
				test.SkipReason = reason
			}
		}

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
func sortResults(failed, skipped, passed *[]types.TestResult) {
	sortFunc := func(i, j int, slice []types.TestResult) bool {
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
