package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTestEvent(t *testing.T) {
	// Test TestEvent struct and JSON marshaling
	event := TestEvent{
		Time:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Action:  "run",
		Package: "github.com/example/pkg",
		Test:    "TestExample",
		Output:  "=== RUN TestExample\n",
		Elapsed: 1.5,
	}

	// Test JSON marshaling
	data, err := json.Marshal(event)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"Action":"run"`)
	assert.Contains(t, string(data), `"Package":"github.com/example/pkg"`)
	assert.Contains(t, string(data), `"Test":"TestExample"`)
	assert.Contains(t, string(data), `"Elapsed":1.5`)

	// Test JSON unmarshaling
	var decoded TestEvent
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, event.Action, decoded.Action)
	assert.Equal(t, event.Package, decoded.Package)
	assert.Equal(t, event.Test, decoded.Test)
	assert.Equal(t, event.Elapsed, decoded.Elapsed)
}

func TestTestResult(t *testing.T) {
	result := TestResult{
		Package:    "github.com/example/pkg",
		Test:       "TestFoo",
		Status:     "pass",
		Duration:   2.5,
		SkipReason: "",
	}

	assert.Equal(t, "github.com/example/pkg", result.Package)
	assert.Equal(t, "TestFoo", result.Test)
	assert.Equal(t, "pass", result.Status)
	assert.Equal(t, 2.5, result.Duration)
	assert.Empty(t, result.SkipReason)

	// Test with skip reason
	skippedResult := TestResult{
		Package:    "github.com/example/pkg",
		Test:       "TestBar",
		Status:     "skip",
		Duration:   0.1,
		SkipReason: "Not implemented yet",
	}

	assert.Equal(t, "skip", skippedResult.Status)
	assert.Equal(t, "Not implemented yet", skippedResult.SkipReason)
}

func TestBuildFailure(t *testing.T) {
	failure := BuildFailure{
		Package: "github.com/example/broken",
		Output:  "undefined: someFunction",
	}

	assert.Equal(t, "github.com/example/broken", failure.Package)
	assert.Equal(t, "undefined: someFunction", failure.Output)
	assert.Contains(t, failure.Output, "undefined")
}

func TestTestSummary(t *testing.T) {
	summary := TestSummary{
		Failed: []TestResult{
			{Package: "pkg1", Test: "TestFail", Status: "fail", Duration: 1.0},
		},
		Skipped: []TestResult{
			{Package: "pkg2", Test: "TestSkip", Status: "skip", Duration: 0.1, SkipReason: "Not ready"},
		},
		Passed: []TestResult{
			{Package: "pkg3", Test: "TestPass", Status: "pass", Duration: 0.5},
			{Package: "pkg3", Test: "TestPass2", Status: "pass", Duration: 0.3},
		},
		BuildFailed: []BuildFailure{
			{Package: "pkg4", Output: "compilation error"},
		},
		Coverage:           "75.5%",
		CoverageData:       &CoverageData{StatementCoverage: "75.5%"},
		TotalElapsedTime:   10.5,
		ExitCodeDiagnostic: "Tests passed but process exited with non-zero code",
	}

	assert.Len(t, summary.Failed, 1)
	assert.Len(t, summary.Skipped, 1)
	assert.Len(t, summary.Passed, 2)
	assert.Len(t, summary.BuildFailed, 1)
	assert.Equal(t, "75.5%", summary.Coverage)
	assert.NotNil(t, summary.CoverageData)
	assert.Equal(t, 10.5, summary.TotalElapsedTime)
	assert.NotEmpty(t, summary.ExitCodeDiagnostic)
}

func TestCoverageFunction(t *testing.T) {
	fn := CoverageFunction{
		File:     "main.go",
		Function: "main",
		Coverage: 85.5,
	}

	assert.Equal(t, "main.go", fn.File)
	assert.Equal(t, "main", fn.Function)
	assert.Equal(t, 85.5, fn.Coverage)
}

func TestCoverageData(t *testing.T) {
	data := CoverageData{
		StatementCoverage: "82.3%",
		FunctionCoverage: []CoverageFunction{
			{File: "main.go", Function: "main", Coverage: 100.0},
			{File: "utils.go", Function: "helper", Coverage: 75.0},
		},
		FilteredFiles: []string{"mock_test.go", "vendor/lib.go"},
	}

	assert.Equal(t, "82.3%", data.StatementCoverage)
	assert.Len(t, data.FunctionCoverage, 2)
	assert.Equal(t, 100.0, data.FunctionCoverage[0].Coverage)
	assert.Equal(t, 75.0, data.FunctionCoverage[1].Coverage)
	assert.Len(t, data.FilteredFiles, 2)
	assert.Contains(t, data.FilteredFiles, "mock_test.go")
}

func TestPackageSummary(t *testing.T) {
	summary := PackageSummary{
		Package:       "github.com/example/pkg",
		TestCount:     10,
		AvgDuration:   0.5,
		TotalDuration: 5.0,
	}

	assert.Equal(t, "github.com/example/pkg", summary.Package)
	assert.Equal(t, 10, summary.TestCount)
	assert.Equal(t, 0.5, summary.AvgDuration)
	assert.Equal(t, 5.0, summary.TotalDuration)
}

func TestCoverageLine(t *testing.T) {
	line := CoverageLine{
		Filename:   "main.go",
		Statements: 100,
		Covered:    85,
	}

	assert.Equal(t, "main.go", line.Filename)
	assert.Equal(t, 100, line.Statements)
	assert.Equal(t, 85, line.Covered)

	// Calculate coverage percentage
	coveragePercent := float64(line.Covered) / float64(line.Statements) * 100
	assert.Equal(t, 85.0, coveragePercent)
}

func TestShortPackage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full github path",
			input:    "github.com/cloudposse/atmos/tools/gotcha",
			expected: "gotcha",
		},
		{
			name:     "simple path",
			input:    "internal/utils",
			expected: "utils",
		},
		{
			name:     "single component",
			input:    "main",
			expected: "main",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "path with trailing slash",
			input:    "pkg/test/",
			expected: "",
		},
		{
			name:     "dotted path",
			input:    "example.com/user/project",
			expected: "project",
		},
		{
			name:     "deeply nested path",
			input:    "a/b/c/d/e/f/g",
			expected: "g",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShortPackage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTestEventAllActions(t *testing.T) {
	// Test various action types
	actions := []string{
		"run",
		"pass",
		"fail",
		"skip",
		"output",
		"pause",
		"cont",
		"bench",
	}

	for _, action := range actions {
		event := TestEvent{
			Time:    time.Now(),
			Action:  action,
			Package: "test/pkg",
		}
		assert.Equal(t, action, event.Action)
	}
}

func TestTestResultStatuses(t *testing.T) {
	// Test various status types
	statuses := []string{
		"pass",
		"fail",
		"skip",
		"",
	}

	for _, status := range statuses {
		result := TestResult{
			Status: status,
		}
		assert.Equal(t, status, result.Status)
	}
}

func TestTestSummaryEmpty(t *testing.T) {
	// Test empty summary
	summary := TestSummary{}

	assert.Empty(t, summary.Failed)
	assert.Empty(t, summary.Skipped)
	assert.Empty(t, summary.Passed)
	assert.Empty(t, summary.BuildFailed)
	assert.Empty(t, summary.Coverage)
	assert.Nil(t, summary.CoverageData)
	assert.Equal(t, 0.0, summary.TotalElapsedTime)
	assert.Empty(t, summary.ExitCodeDiagnostic)
}

func TestCoverageDataNil(t *testing.T) {
	// Test that nil CoverageData is handled properly
	summary := TestSummary{
		CoverageData: nil,
	}

	assert.Nil(t, summary.CoverageData)
}

func TestTestEventWithSubtest(t *testing.T) {
	// Test event with subtest name
	event := TestEvent{
		Time:    time.Now(),
		Action:  "run",
		Package: "github.com/example/pkg",
		Test:    "TestParent/SubTest",
		Output:  "=== RUN TestParent/SubTest\n",
		Elapsed: 0.5,
	}

	assert.Equal(t, "TestParent/SubTest", event.Test)
	assert.Contains(t, event.Test, "/")
}

func TestBuildFailureMultiline(t *testing.T) {
	// Test build failure with multiline output
	failure := BuildFailure{
		Package: "github.com/example/broken",
		Output: `main.go:10:5: undefined: someFunction
main.go:15:10: cannot use x (type int) as type string
main.go:20:1: syntax error: unexpected }`,
	}

	assert.Contains(t, failure.Output, "undefined")
	assert.Contains(t, failure.Output, "cannot use")
	assert.Contains(t, failure.Output, "syntax error")
}

func TestPackageSummaryZeroValues(t *testing.T) {
	// Test package summary with zero values
	summary := PackageSummary{
		Package:       "",
		TestCount:     0,
		AvgDuration:   0.0,
		TotalDuration: 0.0,
	}

	assert.Empty(t, summary.Package)
	assert.Equal(t, 0, summary.TestCount)
	assert.Equal(t, 0.0, summary.AvgDuration)
	assert.Equal(t, 0.0, summary.TotalDuration)
}

func TestCoverageLineFullCoverage(t *testing.T) {
	// Test coverage line with 100% coverage
	line := CoverageLine{
		Filename:   "fully_covered.go",
		Statements: 50,
		Covered:    50,
	}

	assert.Equal(t, line.Statements, line.Covered)
	coveragePercent := float64(line.Covered) / float64(line.Statements) * 100
	assert.Equal(t, 100.0, coveragePercent)
}

func TestCoverageLineNoCoverage(t *testing.T) {
	// Test coverage line with 0% coverage
	line := CoverageLine{
		Filename:   "uncovered.go",
		Statements: 50,
		Covered:    0,
	}

	assert.Equal(t, 0, line.Covered)
	coveragePercent := float64(line.Covered) / float64(line.Statements) * 100
	assert.Equal(t, 0.0, coveragePercent)
}

func TestShortPackageEdgeCases(t *testing.T) {
	// Additional edge cases for ShortPackage
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "only slashes",
			input:    "///",
			expected: "",
		},
		{
			name:     "single slash",
			input:    "/",
			expected: "",
		},
		{
			name:     "path starting with slash",
			input:    "/absolute/path/pkg",
			expected: "pkg",
		},
		{
			name:     "path with spaces",
			input:    "path with spaces/pkg",
			expected: "pkg",
		},
		{
			name:     "unicode path",
			input:    "パス/テスト/パッケージ",
			expected: "パッケージ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShortPackage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}