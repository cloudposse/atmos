package main

import (
	"testing"
)

// Test helper functions functionality
func TestHelperFunctions(t *testing.T) {
	// Test isValidShowFilter
	validValues := []string{"all", "failed", "passed", "skipped"}
	for _, value := range validValues {
		if !isValidShowFilter(value) {
			t.Errorf("isValidShowFilter(%q) should be true", value)
		}
	}

	if isValidShowFilter("invalid") {
		t.Errorf("isValidShowFilter('invalid') should be false")
	}
}

// Test main functions that are called from the new architecture
func TestRunStreamAndParseIntegration(t *testing.T) {
	// Create a test summary for testing output functionality
	summary := &TestSummary{
		Passed: []TestResult{
			{Package: "test/pkg", Test: "TestPass1", Status: "pass", Duration: 0.5},
		},
		Failed: []TestResult{
			{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0},
		},
		Coverage: "75.0%",
	}

	// Test different output formats
	formats := []string{formatStdin, formatMarkdown}

	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			err := handleOutput(summary, format, "")
			if err != nil {
				t.Errorf("handleOutput() with format %s failed: %v", format, err)
			}
		})
	}
}

func TestHandleOutputWithFailedSummary(t *testing.T) {
	// Test that console output works with failed tests.
	summary := &TestSummary{
		Failed: []TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
	}

	err := handleOutput(summary, formatStdin, "")
	if err != nil {
		t.Errorf("handleOutput() with failed tests = %v, want nil", err)
	}
}

func TestConstants(t *testing.T) {
	// Test that constants are properly defined.
	if formatStdin != "stdin" {
		t.Errorf("formatStdin = %v, want 'stdin'", formatStdin)
	}
	if formatMarkdown != "markdown" {
		t.Errorf("formatMarkdown = %v, want 'markdown'", formatMarkdown)
	}
	if formatGitHub != "github" {
		t.Errorf("formatGitHub = %v, want 'github'", formatGitHub)
	}
	if formatBoth != "both" {
		t.Errorf("formatBoth = %v, want 'both'", formatBoth)
	}
	if formatStream != "stream" {
		t.Errorf("formatStream = %v, want 'stream'", formatStream)
	}
	if stdinMarker != "-" {
		t.Errorf("stdinMarker = %v, want '-'", stdinMarker)
	}
	if stdoutPath != "stdout" {
		t.Errorf("stdoutPath = %v, want 'stdout'", stdoutPath)
	}
	if defaultSummaryFile != "test-summary.md" {
		t.Errorf("defaultSummaryFile = %v, want 'test-summary.md'", defaultSummaryFile)
	}
	if coverageHighThreshold != 80.0 {
		t.Errorf("coverageHighThreshold = %v, want 80.0", coverageHighThreshold)
	}
	if coverageMedThreshold != 40.0 {
		t.Errorf("coverageMedThreshold = %v, want 40.0", coverageMedThreshold)
	}
	if base10BitSize != 64 {
		t.Errorf("base10BitSize = %v, want 64", base10BitSize)
	}
}
