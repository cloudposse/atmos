package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

func TestWritePassedTestsTableEnhanced(t *testing.T) {
	tests := []struct {
		name     string
		passed   []types.TestResult
		wantText []string
	}{
		{
			name:     "empty passed tests",
			passed:   []types.TestResult{},
			wantText: []string{}, // Empty tests don't produce output
		},
		{
			name: "single passed test",
			passed: []types.TestResult{
				{Package: "pkg/utils", Test: "TestHelper", Status: "pass", Duration: 0.5},
			},
			wantText: []string{
				"### ✅ Passed Tests (1)",
				"TestHelper",
				"utils", // shortPackage returns just the last part
				"0.50s",
				"<summary>Click to show all passing tests</summary>",
			},
		},
		{
			name: "multiple passed tests",
			passed: []types.TestResult{
				{Package: "pkg/utils", Test: "TestHelper1", Status: "pass", Duration: 0.3},
				{Package: "pkg/main", Test: "TestMain", Status: "pass", Duration: 0.8},
			},
			wantText: []string{
				"### ✅ Passed Tests (2)",
				"TestHelper1",
				"TestMain",
				"<summary>Click to show all passing tests</summary>",
			},
		},
		{
			name:   "passed tests with long list",
			passed: make([]types.TestResult, 300), // More than maxTotalTestsShown
			wantText: []string{
				"### ✅ Passed Tests (300)",
				"Showing", // Should mention how many tests are shown
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fill test data for long list test
			if len(tt.passed) == 300 {
				for i := range tt.passed {
					tt.passed[i] = types.TestResult{
						Package:  "pkg/test",
						Test:     "TestExample" + string(rune('A'+i%26)),
						Status:   "pass",
						Duration: 0.1,
					}
				}
			}

			var buf bytes.Buffer
			WritePassedTestsTable(&buf, tt.passed)

			output := buf.String()

			for _, wantText := range tt.wantText {
				if !strings.Contains(output, wantText) {
					t.Errorf("writePassedTests() output should contain %q, got:\n%s", wantText, output)
				}
			}
		})
	}
}

func TestWriteTestTableEnhanced(t *testing.T) {
	tests := []struct {
		name            string
		tests           []types.TestResult
		includeDuration bool
		totalDuration   float64
		wantText        []string
	}{
		{
			name: "table with duration included",
			tests: []types.TestResult{
				{Package: "pkg/utils", Test: "TestFast", Status: "pass", Duration: 0.1},
				{Package: "pkg/main", Test: "TestSlow", Status: "fail", Duration: 2.5},
			},
			includeDuration: true,
			totalDuration:   2.6,
			wantText: []string{
				"| Test | Package | Duration | % of Total |",
				"TestFast",
				"TestSlow",
				"0.10s",
				"2.50s",
				"3.8%",  // 0.1/2.6 * 100
				"96.2%", // 2.5/2.6 * 100
			},
		},
		{
			name: "table without duration",
			tests: []types.TestResult{
				{Package: "pkg/utils", Test: "TestExample", Status: "fail", Duration: 0.5},
			},
			includeDuration: false,
			totalDuration:   0.0,
			wantText: []string{
				"| Test | Package |",
				"TestExample",
				"utils", // shortPackage returns just the last part
			},
		},
		{
			name:            "empty tests table",
			tests:           []types.TestResult{},
			includeDuration: true,
			totalDuration:   0.0,
			wantText:        []string{}, // Should produce minimal output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeTestTable(&buf, tt.tests, tt.includeDuration, tt.totalDuration)

			output := buf.String()

			for _, wantText := range tt.wantText {
				if !strings.Contains(output, wantText) {
					t.Errorf("writeTestTable() output should contain %q, got:\n%s", wantText, output)
				}
			}
		})
	}
}

func TestWriteDetailedCoverageEnhanced(t *testing.T) {
	tests := []struct {
		name         string
		coverageData *types.CoverageData
		wantText     []string
	}{
		{
			name: "comprehensive coverage data",
			coverageData: &types.CoverageData{
				StatementCoverage: "85.5%",
				FunctionCoverage: []types.CoverageFunction{
					{Function: "main", File: "main.go", Coverage: 100.0},
					{Function: "helper", File: "utils.go", Coverage: 0.0},
				},
			},
			wantText: []string{
				"## 📊 Test Coverage",
				"85.5%",
				"🟢", // High coverage emoji
				"Statement Coverage",
				"Function Coverage",
			},
		},
		{
			name: "medium coverage data",
			coverageData: &types.CoverageData{
				StatementCoverage: "65.0%",
				FunctionCoverage: []types.CoverageFunction{
					{Function: "test1", File: "test.go", Coverage: 50.0},
					{Function: "test2", File: "test.go", Coverage: 80.0},
				},
			},
			wantText: []string{
				"## 📊 Test Coverage",
				"65.0%",
				"🟡", // Medium coverage emoji
			},
		},
		{
			name: "low coverage data",
			coverageData: &types.CoverageData{
				StatementCoverage: "25.0%",
				FunctionCoverage: []types.CoverageFunction{
					{Function: "uncovered", File: "test.go", Coverage: 0.0},
				},
			},
			wantText: []string{
				"## 📊 Test Coverage",
				"25.0%",
				"🔴", // Low coverage emoji
			},
		},
		{
			name:         "nil coverage data",
			coverageData: nil,
			wantText:     []string{}, // Should handle nil gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteDetailedCoverage(&buf, tt.coverageData)

			output := buf.String()

			for _, wantText := range tt.wantText {
				if !strings.Contains(output, wantText) {
					t.Errorf("writeTestCoverageSection() output should contain %q, got:\n%s", wantText, output)
				}
			}
		})
	}
}

func TestGetUncoveredFunctionsInPREnhanced(t *testing.T) {
	tests := []struct {
		name          string
		functions     []types.CoverageFunction
		changedFiles  []string
		wantCount     int
		wantFunctions []string
	}{
		{
			name: "functions in changed files",
			functions: []types.CoverageFunction{
				{Function: "ChangedFunc", File: "changed.go", Coverage: 0.0},
				{Function: "UnchangedFunc", File: "unchanged.go", Coverage: 0.0},
				{Function: "CoveredFunc", File: "changed.go", Coverage: 100.0},
			},
			changedFiles:  []string{"changed.go"},
			wantCount:     2, // Total functions in changed files (1 uncovered + 1 covered)
			wantFunctions: []string{"ChangedFunc"},
		},
		{
			name: "no changed files",
			functions: []types.CoverageFunction{
				{Function: "Func1", File: "test.go", Coverage: 0.0},
			},
			changedFiles:  []string{},
			wantCount:     0,
			wantFunctions: []string{},
		},
		{
			name: "all functions covered",
			functions: []types.CoverageFunction{
				{Function: "CoveredFunc", File: "changed.go", Coverage: 100.0},
			},
			changedFiles:  []string{"changed.go"},
			wantCount:     1, // Total functions in changed files (even though all are covered)
			wantFunctions: []string{},
		},
		{
			name:          "empty functions list",
			functions:     []types.CoverageFunction{},
			changedFiles:  []string{"changed.go"},
			wantCount:     0,
			wantFunctions: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFunctions, gotCount := getUncoveredFunctionsInPR(tt.functions, tt.changedFiles)

			if gotCount != tt.wantCount {
				t.Errorf("getUncoveredFunctionsInPR() count = %v, want %v", gotCount, tt.wantCount)
			}

			if len(gotFunctions) != len(tt.wantFunctions) {
				t.Errorf("getUncoveredFunctionsInPR() returned %d functions, want %d", len(gotFunctions), len(tt.wantFunctions))
				return
			}

			for i, wantFunc := range tt.wantFunctions {
				if gotFunctions[i].Function != wantFunc {
					t.Errorf("getUncoveredFunctionsInPR() function[%d] = %v, want %v", i, gotFunctions[i].Function, wantFunc)
				}
			}
		})
	}
}
