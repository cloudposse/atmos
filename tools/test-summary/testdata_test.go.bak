package main

import (
	"strings"
	"testing"
)

// Test data constants for use across test files.
const (
	// Simple test event with passing test.
	simplePassJSON = `{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"github.com/test/pkg","Test":"TestExample","Elapsed":0.5}`

	// Test event with failing test.
	simpleFailJSON = `{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"github.com/test/pkg","Test":"TestFailing","Elapsed":1.2}`

	// Test event with skipped test.
	simpleSkipJSON = `{"Time":"2024-01-01T00:00:00Z","Action":"skip","Package":"github.com/test/pkg","Test":"TestSkipped"}`

	// Coverage output line.
	coverageOutputJSON = `{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"github.com/test/pkg","Output":"coverage: 75.5% of statements\n"}`

	// Regular output line.
	regularOutputJSON = `{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"github.com/test/pkg","Test":"TestExample","Output":"=== RUN   TestExample\n"}`

	// Complete test sequence.
	completeTestSequence = `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"github.com/test/pkg","Test":"TestExample"}
{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"github.com/test/pkg","Test":"TestExample","Output":"=== RUN   TestExample\n"}
{"Time":"2024-01-01T00:00:02Z","Action":"output","Package":"github.com/test/pkg","Test":"TestExample","Output":"test output\n"}
{"Time":"2024-01-01T00:00:03Z","Action":"pass","Package":"github.com/test/pkg","Test":"TestExample","Elapsed":1.0}
{"Time":"2024-01-01T00:00:04Z","Action":"output","Package":"github.com/test/pkg","Output":"PASS\n"}
{"Time":"2024-01-01T00:00:05Z","Action":"output","Package":"github.com/test/pkg","Output":"coverage: 82.3% of statements\n"}
{"Time":"2024-01-01T00:00:06Z","Action":"pass","Package":"github.com/test/pkg","Elapsed":2.0}`

	// Mixed content with plain text and JSON.
	mixedContent = `This is plain text that should be passed through
{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"github.com/test/pkg","Test":"TestOne","Elapsed":0.1}
Another plain text line
{"Time":"2024-01-01T00:00:01Z","Action":"fail","Package":"github.com/test/pkg","Test":"TestTwo","Elapsed":0.2}
{"Time":"2024-01-01T00:00:02Z","Action":"skip","Package":"github.com/test/pkg","Test":"TestThree"}
More plain text`

	// Multiple packages test data.
	multiPackageJSON = `{"Action":"pass","Package":"github.com/test/cmd","Test":"TestCmd","Elapsed":0.1}
{"Action":"pass","Package":"github.com/test/pkg","Test":"TestPkg","Elapsed":0.2}
{"Action":"fail","Package":"github.com/test/internal","Test":"TestInternal","Elapsed":0.3}`
)

// Helper function to create a test summary with specified counts.
func createTestSummary(passed, failed, skipped int, coverage string) *TestSummary {
	summary := &TestSummary{
		Coverage: coverage,
	}

	for i := 0; i < passed; i++ {
		summary.Passed = append(summary.Passed, TestResult{
			Package:  "test/pkg",
			Test:     "TestPass" + string(rune('A'+i)),
			Status:   "pass",
			Duration: 0.1 * float64(i+1),
		})
	}

	for i := 0; i < failed; i++ {
		summary.Failed = append(summary.Failed, TestResult{
			Package:  "test/pkg",
			Test:     "TestFail" + string(rune('A'+i)),
			Status:   "fail",
			Duration: 0.2 * float64(i+1),
		})
		summary.ExitCode = 1
	}

	for i := 0; i < skipped; i++ {
		summary.Skipped = append(summary.Skipped, TestResult{
			Package: "test/pkg",
			Test:    "TestSkip" + string(rune('A'+i)),
			Status:  "skip",
		})
	}

	return summary
}

// Helper function to check if a string contains all expected substrings.
func containsAll(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output missing expected substring %q\nGot:\n%s", w, got)
		}
	}
}

// Helper function to assert test counts.
func assertTestCounts(t *testing.T, summary *TestSummary, wantPassed, wantFailed, wantSkipped int) {
	t.Helper()
	if got := len(summary.Passed); got != wantPassed {
		t.Errorf("Passed tests: got %d, want %d", got, wantPassed)
	}
	if got := len(summary.Failed); got != wantFailed {
		t.Errorf("Failed tests: got %d, want %d", got, wantFailed)
	}
	if got := len(summary.Skipped); got != wantSkipped {
		t.Errorf("Skipped tests: got %d, want %d", got, wantSkipped)
	}
}

// Helper function to assert coverage value.
func assertCoverage(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("Coverage: got %q, want %q", got, want)
	}
}

// Helper function to assert exit code.
func assertExitCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("Exit code: got %d, want %d", got, want)
	}
}
