package main

import (
	"os"
	"testing"
)

func TestMain_Integration(t *testing.T) {
	// This test verifies the main function integration.
	// We'll test by setting up args and capturing exit behavior.

	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{
			name:     "invalid format",
			args:     []string{"test-summary", "-format=invalid"},
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args.
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set test args.
			os.Args = tt.args

			// Test run function directly to avoid os.Exit.
			exitCode := run("non-existent-file.json", "invalid", "", "", false)

			if exitCode != tt.wantExit {
				t.Errorf("run() exitCode = %v, want %v", exitCode, tt.wantExit)
			}
		})
	}
}

func TestRun_AdditionalCases(t *testing.T) {
	tests := []struct {
		name       string
		inputFile  string
		format     string
		outputFile string
		wantExit   int
	}{
		{
			name:       "invalid input file",
			inputFile:  "/non/existent/file.json",
			format:     formatStdin,
			outputFile: "",
			wantExit:   1,
		},
		{
			name:       "valid console format with stdin",
			inputFile:  "-",
			format:     formatStdin,
			outputFile: "",
			wantExit:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(tt.inputFile, tt.format, tt.outputFile, "", false)
			if got != tt.wantExit {
				t.Errorf("run() = %v, want %v", got, tt.wantExit)
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
	if formatStdin != "console" {
		t.Errorf("formatStdin = %v, want 'console'", formatStdin)
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
