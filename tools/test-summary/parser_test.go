package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestParseTestJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPassed  int
		wantFailed  int
		wantSkipped int
		wantCoverage string
		wantConsole string
	}{
		{
			name:       "single passing test",
			input:      simplePassJSON,
			wantPassed: 1,
		},
		{
			name:       "single failing test",
			input:      simpleFailJSON,
			wantFailed: 1,
		},
		{
			name:        "single skipped test",
			input:       simpleSkipJSON,
			wantSkipped: 1,
		},
		{
			name:         "complete test sequence with coverage",
			input:        completeTestSequence,
			wantPassed:   1,
			wantCoverage: "82.3%",
			wantConsole:  "=== RUN   TestExample\ntest output\nPASS\ncoverage: 82.3% of statements\n",
		},
		{
			name:        "mixed content with plain text",
			input:       mixedContent,
			wantPassed:  1,
			wantFailed:  1,
			wantSkipped: 1,
			wantConsole: "This is plain text that should be passed through\nAnother plain text line\nMore plain text\n",
		},
		{
			name:       "multiple packages",
			input:      multiPackageJSON,
			wantPassed: 2,
			wantFailed: 1,
		},
		{
			name:  "empty input",
			input: "",
		},
		{
			name:        "only plain text",
			input:       "This is not JSON\nNeither is this\n",
			wantConsole: "This is not JSON\nNeither is this\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			summary, console := parseTestJSON(reader)

			assertTestCounts(t, summary, tt.wantPassed, tt.wantFailed, tt.wantSkipped)
			assertCoverage(t, summary.Coverage, tt.wantCoverage)

			if tt.wantConsole != "" && console != tt.wantConsole {
				t.Errorf("Console output mismatch\nGot:\n%s\nWant:\n%s", console, tt.wantConsole)
			}

			// Check exit code is set for failed tests.
			if tt.wantFailed > 0 {
				assertExitCode(t, summary.ExitCode, 1)
			} else {
				assertExitCode(t, summary.ExitCode, 0)
			}
		})
	}
}

func TestProcessLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantConsole   string
		wantCoverage  string
		wantTestAdded bool
	}{
		{
			name:        "plain text line",
			line:        "This is plain text",
			wantConsole: "This is plain text\n",
		},
		{
			name:        "output event with coverage",
			line:        coverageOutputJSON,
			wantConsole: "coverage: 75.5% of statements\n",
			wantCoverage: "75.5%",
		},
		{
			name:        "regular output event",
			line:        regularOutputJSON,
			wantConsole: "=== RUN   TestExample\n",
		},
		{
			name:          "pass test event",
			line:          simplePassJSON,
			wantTestAdded: true,
		},
		{
			name:          "fail test event",
			line:          simpleFailJSON,
			wantTestAdded: true,
		},
		{
			name:          "skip test event",
			line:          simpleSkipJSON,
			wantTestAdded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var console strings.Builder
			results := make(map[string]*TestResult)
			summary := &TestSummary{}
			coverageRe := regexp.MustCompile(`coverage:\s+([\d.]+)%\s+of\s+statements`)

			processLine(tt.line, &console, results, summary, coverageRe)

			if console.String() != tt.wantConsole {
				t.Errorf("Console output: got %q, want %q", console.String(), tt.wantConsole)
			}

			if summary.Coverage != tt.wantCoverage {
				t.Errorf("Coverage: got %q, want %q", summary.Coverage, tt.wantCoverage)
			}

			if tt.wantTestAdded && len(results) == 0 {
				t.Error("Expected test result to be added, but none found")
			}
		})
	}
}

func TestRecordTestResult(t *testing.T) {
	tests := []struct {
		name         string
		event        TestEvent
		wantExitCode int
	}{
		{
			name: "passing test",
			event: TestEvent{
				Action:  "pass",
				Package: "test/pkg",
				Test:    "TestPass",
				Elapsed: 0.5,
			},
			wantExitCode: 0,
		},
		{
			name: "failing test",
			event: TestEvent{
				Action:  "fail",
				Package: "test/pkg",
				Test:    "TestFail",
				Elapsed: 1.0,
			},
			wantExitCode: 1,
		},
		{
			name: "skipped test",
			event: TestEvent{
				Action:  "skip",
				Package: "test/pkg",
				Test:    "TestSkip",
				Elapsed: 0.0,
			},
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make(map[string]*TestResult)
			summary := &TestSummary{}

			recordTestResult(&tt.event, results, summary)

			key := tt.event.Package + "/" + tt.event.Test
			result, exists := results[key]
			if !exists {
				t.Fatal("Expected result not found in map")
			}

			if result.Status != tt.event.Action {
				t.Errorf("Status: got %q, want %q", result.Status, tt.event.Action)
			}
			if result.Package != tt.event.Package {
				t.Errorf("Package: got %q, want %q", result.Package, tt.event.Package)
			}
			if result.Test != tt.event.Test {
				t.Errorf("Test: got %q, want %q", result.Test, tt.event.Test)
			}
			if result.Duration != tt.event.Elapsed {
				t.Errorf("Duration: got %v, want %v", result.Duration, tt.event.Elapsed)
			}

			assertExitCode(t, summary.ExitCode, tt.wantExitCode)
		})
	}
}

func TestCategorizeResults(t *testing.T) {
	results := map[string]*TestResult{
		"pkg/TestPass1": {Package: "pkg", Test: "TestPass1", Status: "pass"},
		"pkg/TestPass2": {Package: "pkg", Test: "TestPass2", Status: "pass"},
		"pkg/TestFail1": {Package: "pkg", Test: "TestFail1", Status: "fail"},
		"pkg/TestSkip1": {Package: "pkg", Test: "TestSkip1", Status: "skip"},
		"pkg/TestSkip2": {Package: "pkg", Test: "TestSkip2", Status: "skip"},
	}

	summary := &TestSummary{}
	categorizeResults(results, summary)

	assertTestCounts(t, summary, 2, 1, 2)

	// Verify correct categorization.
	if summary.Passed[0].Test != "TestPass1" && summary.Passed[0].Test != "TestPass2" {
		t.Errorf("Unexpected passed test: %s", summary.Passed[0].Test)
	}
	if summary.Failed[0].Test != "TestFail1" {
		t.Errorf("Unexpected failed test: %s", summary.Failed[0].Test)
	}
	if summary.Skipped[0].Test != "TestSkip1" && summary.Skipped[0].Test != "TestSkip2" {
		t.Errorf("Unexpected skipped test: %s", summary.Skipped[0].Test)
	}
}

func TestSortResults(t *testing.T) {
	summary := &TestSummary{
		Passed: []TestResult{
			{Package: "pkg/b", Test: "TestB"},
			{Package: "pkg/a", Test: "TestC"},
			{Package: "pkg/a", Test: "TestA"},
		},
		Failed: []TestResult{
			{Package: "pkg/z", Test: "TestZ"},
			{Package: "pkg/y", Test: "TestY"},
		},
		Skipped: []TestResult{
			{Package: "pkg/m", Test: "TestN"},
			{Package: "pkg/m", Test: "TestM"},
		},
	}

	sortResults(summary)

	// Check passed tests are sorted.
	if summary.Passed[0].Package != "pkg/a" || summary.Passed[0].Test != "TestA" {
		t.Errorf("First passed test: got %s/%s, want pkg/a/TestA",
			summary.Passed[0].Package, summary.Passed[0].Test)
	}
	if summary.Passed[1].Package != "pkg/a" || summary.Passed[1].Test != "TestC" {
		t.Errorf("Second passed test: got %s/%s, want pkg/a/TestC",
			summary.Passed[1].Package, summary.Passed[1].Test)
	}
	if summary.Passed[2].Package != "pkg/b" || summary.Passed[2].Test != "TestB" {
		t.Errorf("Third passed test: got %s/%s, want pkg/b/TestB",
			summary.Passed[2].Package, summary.Passed[2].Test)
	}

	// Check failed tests are sorted.
	if summary.Failed[0].Package != "pkg/y" {
		t.Errorf("First failed test package: got %s, want pkg/y", summary.Failed[0].Package)
	}
	if summary.Failed[1].Package != "pkg/z" {
		t.Errorf("Second failed test package: got %s, want pkg/z", summary.Failed[1].Package)
	}

	// Check skipped tests are sorted.
	if summary.Skipped[0].Test != "TestM" {
		t.Errorf("First skipped test: got %s, want TestM", summary.Skipped[0].Test)
	}
	if summary.Skipped[1].Test != "TestN" {
		t.Errorf("Second skipped test: got %s, want TestN", summary.Skipped[1].Test)
	}
}