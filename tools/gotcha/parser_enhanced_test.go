package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestSortResults(t *testing.T) {
	tests := []struct {
		name        string
		failed      []TestResult
		skipped     []TestResult  
		passed      []TestResult
		wantOrder   string // Description of expected order
	}{
		{
			name: "sort by duration descending",
			failed: []TestResult{
				{Test: "TestFast", Duration: 0.1},
				{Test: "TestSlow", Duration: 2.0},
				{Test: "TestMedium", Duration: 0.5},
			},
			skipped: []TestResult{
				{Test: "TestSkippedSlow", Duration: 1.0},
				{Test: "TestSkippedFast", Duration: 0.2},
			},
			passed: []TestResult{
				{Test: "TestPassedSlow", Duration: 3.0},
				{Test: "TestPassedFast", Duration: 0.3},
			},
			wantOrder: "slowest first",
		},
		{
			name:     "empty results",
			failed:   []TestResult{},
			skipped:  []TestResult{},
			passed:   []TestResult{},
			wantOrder: "empty",
		},
		{
			name: "single items in each category",
			failed: []TestResult{
				{Test: "TestFailedOne", Duration: 0.5},
			},
			skipped: []TestResult{
				{Test: "TestSkippedOne", Duration: 0.3},
			},
			passed: []TestResult{
				{Test: "TestPassedOne", Duration: 0.8},
			},
			wantOrder: "single items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make copies to avoid modifying test data
			failed := make([]TestResult, len(tt.failed))
			copy(failed, tt.failed)
			skipped := make([]TestResult, len(tt.skipped))
			copy(skipped, tt.skipped)
			passed := make([]TestResult, len(tt.passed))
			copy(passed, tt.passed)

			sortResults(&failed, &skipped, &passed)
			
			// Verify failed results are sorted by duration (descending)
			for i := 1; i < len(failed); i++ {
				if failed[i-1].Duration < failed[i].Duration {
					t.Errorf("sortResults() failed tests not sorted by duration: %v should be >= %v", 
						failed[i-1].Duration, failed[i].Duration)
				}
			}
			
			// Verify skipped results are sorted by duration (descending)
			for i := 1; i < len(skipped); i++ {
				if skipped[i-1].Duration < skipped[i].Duration {
					t.Errorf("sortResults() skipped tests not sorted by duration: %v should be >= %v", 
						skipped[i-1].Duration, skipped[i].Duration)
				}
			}
			
			// Verify passed results are sorted by duration (descending)  
			for i := 1; i < len(passed); i++ {
				if passed[i-1].Duration < passed[i].Duration {
					t.Errorf("sortResults() passed tests not sorted by duration: %v should be >= %v", 
						passed[i-1].Duration, passed[i].Duration)
				}
			}
		})
	}
}

func TestParseTestJSONEnhanced(t *testing.T) {
	tests := []struct {
		name         string
		jsonInput    string
		coverProfile string
		excludeMocks bool
		wantErr      bool
		checkResult  func(*TestSummary) error
	}{
		{
			name: "comprehensive test results with coverage",
			jsonInput: `{"Time":"2023-01-01T00:00:00Z","Action":"output","Package":"example/pkg","Output":"coverage: 85.0% of statements\n"}
{"Time":"2023-01-01T00:00:01Z","Action":"pass","Package":"example/pkg","Test":"TestPass","Elapsed":0.5}
{"Time":"2023-01-01T00:00:02Z","Action":"fail","Package":"example/pkg","Test":"TestFail","Elapsed":1.0}
{"Time":"2023-01-01T00:00:03Z","Action":"skip","Package":"example/pkg","Test":"TestSkip","Elapsed":0.1}`,
			coverProfile: "",
			excludeMocks: true,
			wantErr:      false,
			checkResult: func(summary *TestSummary) error {
				if len(summary.Passed) != 1 {
					return fmt.Errorf("expected 1 passed test, got %d", len(summary.Passed))
				}
				if len(summary.Failed) != 1 {
					return fmt.Errorf("expected 1 failed test, got %d", len(summary.Failed))
				}
				if len(summary.Skipped) != 1 {
					return fmt.Errorf("expected 1 skipped test, got %d", len(summary.Skipped))
				}
				if summary.Coverage != "85.0%" {
					return fmt.Errorf("expected coverage '85.0%%', got %q", summary.Coverage)
				}
				return nil
			},
		},
		{
			name: "malformed JSON with recovery",
			jsonInput: `{"Time":"2023-01-01T00:00:00Z","Action":"pass","Package":"example/pkg","Test":"TestPass"}
invalid json line that should be skipped
{"Time":"2023-01-01T00:00:01Z","Action":"fail","Package":"example/pkg","Test":"TestFail"}`,
			coverProfile: "",
			excludeMocks: true,
			wantErr:      false,
			checkResult: func(summary *TestSummary) error {
				if len(summary.Passed) != 1 {
					return fmt.Errorf("expected 1 passed test despite malformed JSON, got %d", len(summary.Passed))
				}
				if len(summary.Failed) != 1 {
					return fmt.Errorf("expected 1 failed test despite malformed JSON, got %d", len(summary.Failed))
				}
				return nil
			},
		},
		{
			name: "test with coverage file",
			jsonInput: `{"Time":"2023-01-01T00:00:00Z","Action":"pass","Package":"example/pkg","Test":"TestWithCoverage"}`,
			coverProfile: "test.out", // Use our test coverage file
			excludeMocks: true,
			wantErr:      false,
			checkResult: func(summary *TestSummary) error {
				if summary.CoverageData == nil {
					return fmt.Errorf("expected coverage data to be parsed")
				}
				if summary.CoverageData.StatementCoverage == "" {
					return fmt.Errorf("expected statement coverage to be calculated")
				}
				return nil
			},
		},
		{
			name:         "empty input",
			jsonInput:    "",
			coverProfile: "",
			excludeMocks: true,
			wantErr:      false,
			checkResult: func(summary *TestSummary) error {
				if len(summary.Passed) != 0 || len(summary.Failed) != 0 || len(summary.Skipped) != 0 {
					return fmt.Errorf("expected empty results for empty input")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.jsonInput)
			
			summary, err := parseTestJSON(reader, tt.coverProfile, tt.excludeMocks)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && tt.checkResult != nil {
				if checkErr := tt.checkResult(summary); checkErr != nil {
					t.Errorf("parseTestJSON() result validation failed: %v", checkErr)
				}
			}
		})
	}
}

// Test edge cases in processLine
func TestProcessLineEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "coverage line extraction",
			line:     "coverage: 85.5% of statements",
			expected: "85.5%",
		},
		{
			name:     "coverage line with extra text",
			line:     "    coverage: 42.0% of statements in github.com/example/pkg",
			expected: "42.0%",
		},
		{
			name:     "non-coverage line",
			line:     "=== RUN   TestExample",
			expected: "",
		},
		{
			name:     "empty line",
			line:     "",
			expected: "",
		},
		{
			name:     "coverage line with no percentage",
			line:     "coverage: statements",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tests := make(map[string]TestResult)
			
			result := processLine(tt.line, tests)
			
			if result != tt.expected {
				t.Errorf("processLine(%q) = %q, want %q", tt.line, result, tt.expected)
			}
		})
	}
}

// Test edge cases in extractCoverage
func TestExtractCoverageEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "multiple coverage lines - should use last",
			output:   "coverage: 70.0% of statements\ncoverage: 85.0% of statements\n",
			expected: "85.0%",
		},
		{
			name:     "coverage with decimal places",
			output:   "coverage: 67.89% of statements",
			expected: "67.89%",
		},
		{
			name:     "no coverage information",
			output:   "PASS\nok  \texample/pkg\t0.123s\n",
			expected: "",
		},
		{
			name:     "coverage at 100%",
			output:   "coverage: 100.0% of statements",
			expected: "100.0%",
		},
		{
			name:     "coverage at 0%",
			output:   "coverage: 0.0% of statements",
			expected: "0.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCoverage(tt.output)
			
			if result != tt.expected {
				t.Errorf("extractCoverage(%q) = %q, want %q", tt.output, result, tt.expected)
			}
		})
	}
}