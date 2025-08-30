package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteFailedTests(t *testing.T) {
	tests := []struct {
		name   string
		failed []TestResult
		want   []string
	}{
		{
			name:   "no failed tests",
			failed: []TestResult{},
			want:   []string{}, // Empty section should be hidden
		},
		{
			name: "single failed test",
			failed: []TestResult{
				{Package: "github.com/test/pkg", Test: "TestExample", Status: "fail", Duration: 1.5},
			},
			want: []string{
				"### ❌ Failed Tests (1)",
				"| Test | Package | Duration |",
				"| `TestExample` | pkg | 1.50s |",
				"**Run locally to reproduce:**",
				"go test github.com/test/pkg -run ^TestExample$ -v",
			},
		},
		{
			name: "multiple failed tests",
			failed: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestA", Status: "fail", Duration: 0.5},
				{Package: "github.com/test/pkg2", Test: "TestB", Status: "fail", Duration: 2.0},
			},
			want: []string{
				"### ❌ Failed Tests (2)",
				"| `TestA` | pkg1 | 0.50s |",
				"| `TestB` | pkg2 | 2.00s |",
				"go test github.com/test/pkg1 -run ^TestA$ -v",
				"go test github.com/test/pkg2 -run ^TestB$ -v",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeFailedTests(&buf, tt.failed)
			output := buf.String()

			if len(tt.want) == 0 {
				if output != "" {
					t.Errorf("writeFailedTests() expected no output, got: %s", output)
				}
				return
			}

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("writeFailedTests() missing expected content: %s\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestWriteSkippedTests(t *testing.T) {
	tests := []struct {
		name    string
		skipped []TestResult
		want    []string
	}{
		{
			name:    "no skipped tests",
			skipped: []TestResult{},
			want:    []string{},
		},
		{
			name: "single skipped test",
			skipped: []TestResult{
				{Package: "github.com/test/pkg", Test: "TestSkipped", Status: "skip"},
			},
			want: []string{
				"### ⏭️ Skipped Tests (1)",
				"| Test | Package |",
				"| `TestSkipped` | pkg |",
			},
		},
		{
			name: "multiple skipped tests",
			skipped: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestSkipA", Status: "skip"},
				{Package: "github.com/test/pkg2", Test: "TestSkipB", Status: "skip"},
			},
			want: []string{
				"### ⏭️ Skipped Tests (2)",
				"| `TestSkipA` | pkg1 |",
				"| `TestSkipB` | pkg2 |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeSkippedTests(&buf, tt.skipped)
			output := buf.String()

			if len(tt.want) == 0 {
				if output != "" {
					t.Errorf("writeSkippedTests() expected no output, got: %s", output)
				}
				return
			}

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("writeSkippedTests() missing expected content: %s\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestWritePassedTests(t *testing.T) {
	tests := []struct {
		name   string
		passed []TestResult
		want   []string
	}{
		{
			name:   "no passed tests",
			passed: []TestResult{},
			want:   []string{},
		},
		{
			name: "single passed test",
			passed: []TestResult{
				{Package: "github.com/test/pkg", Test: "TestPassed", Status: "pass", Duration: 0.3},
			},
			want: []string{
				"### ✅ Passed Tests (1)",
				"<details>",
				"<summary>Click to show all passing tests</summary>",
				"| Test | Package | Duration |",
				"| `TestPassed` | pkg | 0.30s |",
				"</details>",
			},
		},
		{
			name: "multiple passed tests",
			passed: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestPassA", Status: "pass", Duration: 0.1},
				{Package: "github.com/test/pkg2", Test: "TestPassB", Status: "pass", Duration: 0.8},
			},
			want: []string{
				"### ✅ Passed Tests (2)",
				"| `TestPassA` | pkg1 | 0.10s |",
				"| `TestPassB` | pkg2 | 0.80s |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writePassedTests(&buf, tt.passed)
			output := buf.String()

			if len(tt.want) == 0 {
				if output != "" {
					t.Errorf("writePassedTests() expected no output, got: %s", output)
				}
				return
			}

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("writePassedTests() missing expected content: %s\nGot:\n%s", want, output)
				}
			}
		})
	}
}

// Note: TestWriteCoverageSection and TestShortPackage already exist in output_test.go
