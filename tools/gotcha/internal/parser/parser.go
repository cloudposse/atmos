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
	buildFailures := make(map[string]*types.BuildFailure) // Track build failures
	var coverage string
	var coverageData *types.CoverageData
	var totalElapsedTime float64

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		testCoverage, pkgElapsed, buildFail := processLineWithElapsedSkipAndBuild(line, tests, skipReasons, buildFailures)
		if testCoverage != "" {
			coverage = testCoverage
		}
		// Track the maximum package elapsed time (overall test run time)
		if pkgElapsed > totalElapsedTime {
			totalElapsedTime = pkgElapsed
		}
		// Track build failures
		if buildFail != nil {
			buildFailures[buildFail.Package] = buildFail
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

	// Convert build failures map to slice
	var buildFailedList []types.BuildFailure
	for _, bf := range buildFailures {
		buildFailedList = append(buildFailedList, *bf)
	}
	// Sort build failures by package name
	sort.Slice(buildFailedList, func(i, j int) bool {
		return buildFailedList[i].Package < buildFailedList[j].Package
	})

	return &types.TestSummary{
		Failed:           failed,
		Skipped:          skipped,
		Passed:           passed,
		BuildFailed:      buildFailedList,
		Coverage:         coverage,
		CoverageData:     coverageData,
		TotalElapsedTime: totalElapsedTime,
	}, nil
}

// processLineWithElapsedSkipAndBuild processes a single line of JSON output and returns coverage, elapsed time, and build failures.
// It also captures skip reasons from test output and detects build failures.
func processLineWithElapsedSkipAndBuild(line string, tests map[string]types.TestResult, skipReasons map[string]string, buildFailures map[string]*types.BuildFailure) (string, float64, *types.BuildFailure) {
	// Try to parse as JSON.
	var event types.TestEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return "", 0, nil // Skip non-JSON lines.
	}

	var coverage string
	var elapsed float64
	var buildFailure *types.BuildFailure

	// Extract coverage info from output.
	if event.Action == "output" && strings.Contains(event.Output, "coverage:") {
		if cov := extractCoverage(event.Output); cov != "" {
			coverage = cov
		}
	}

	// Detect build failures: package-level fail with no test name and special output
	if event.Test == "" && event.Action == "fail" && event.Package != "" {
		// Check if we already have output for this package indicating build failure
		if bf, exists := buildFailures[event.Package]; exists && bf.Output != "" {
			// Already tracked this build failure
		} else {
			// New build failure detected
			buildFailure = &types.BuildFailure{
				Package: event.Package,
				Output:  "", // Will be filled by subsequent output events
			}
		}
		// Also check the current output for build failure indicators
		if event.Output != "" && strings.Contains(event.Output, "[build failed]") {
			if buildFailure == nil {
				buildFailure = &types.BuildFailure{
					Package: event.Package,
					Output:  event.Output,
				}
			} else {
				buildFailure.Output = event.Output
			}
		}
	}

	// Capture build error output
	if event.Action == "output" && event.Test == "" && event.Package != "" {
		// Check if this package has a build failure
		if bf, exists := buildFailures[event.Package]; exists {
			// Append output to build failure
			bf.Output += event.Output
		} else if strings.Contains(event.Output, "build failed") || strings.Contains(event.Output, "cannot find package") || 
				  strings.Contains(event.Output, "undefined:") || strings.Contains(event.Output, "declared and not used") {
			// This looks like a build error, track it
			buildFailure = &types.BuildFailure{
				Package: event.Package,
				Output:  event.Output,
			}
		}
	}

	// Track skip reasons
	if event.Action == "output" && event.Test != "" {
		key := event.Package + "." + event.Test
		output := strings.TrimSpace(event.Output)
		
		// Check if this is a skip reason output
		if strings.HasPrefix(output, "--- SKIP:") {
			// Mark that we're tracking skip reason for this test
			if _, exists := skipReasons[key]; !exists {
				skipReasons[key] = "" // Will be filled by next line(s)
			}
		} else if strings.Contains(output, ": Skipping") || strings.Contains(output, "t.Skip") {
			// Look for skip reason in current line (before --- SKIP)
			if idx := strings.Index(output, ": "); idx > 0 {
				reason := strings.TrimSpace(output[idx+2:])
				if reason != "" {
					skipReasons[key] = reason
				}
			}
		} else if _, tracking := skipReasons[key]; tracking && skipReasons[key] == "" {
			// This might be the skip reason after --- SKIP line
			reason := output
			if idx := strings.Index(reason, ".go:"); idx > 0 {
				afterFile := reason[idx+4:]
				if colonIdx := strings.Index(afterFile, ": "); colonIdx > 0 {
					reason = strings.TrimSpace(afterFile[colonIdx+2:])
				}
			}
			if reason != "" && reason != output {
				skipReasons[key] = reason
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

	return coverage, elapsed, buildFailure
}

// processLineWithElapsedAndSkipReason processes a single line of JSON output and returns coverage and elapsed time.
// It also captures skip reasons from test output.
func processLineWithElapsedAndSkipReason(line string, tests map[string]types.TestResult, skipReasons map[string]string) (string, float64) {
	// Call the new function but ignore build failures for backward compatibility
	buildFailures := make(map[string]*types.BuildFailure)
	coverage, elapsed, _ := processLineWithElapsedSkipAndBuild(line, tests, skipReasons, buildFailures)
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
