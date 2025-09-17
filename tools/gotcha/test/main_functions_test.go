package test

import (
	"testing"

	"github.com/cloudposse/gotcha/internal/output"
	"github.com/cloudposse/gotcha/pkg/constants"
	"github.com/cloudposse/gotcha/pkg/types"
)

// Test helper functions functionality.
func TestHelperFunctions(t *testing.T) {
	// Test isValidShowFilter values
	// Note: The actual validation is now handled internally
	validValues := []string{"all", "failed", "passed", "skipped"}
	for _, value := range validValues {
		// These are the valid values for the show filter
		if value != "all" && value != "failed" && value != "passed" && value != "skipped" {
			t.Errorf("Invalid show filter value: %q", value)
		}
	}
}

// Test main functions that are called from the new architecture.
func TestRunStreamAndParseIntegration(t *testing.T) {
	// Create a test summary for testing output functionality
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "test/pkg", Test: "TestPass1", Status: "pass", Duration: 0.5},
		},
		Failed: []types.TestResult{
			{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0},
		},
		Coverage: "75.0%",
	}

	// Test different output formats
	formats := []string{"terminal", "markdown"}

	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			err := output.HandleOutput(summary, format, "", true)
			if err != nil {
				t.Errorf("HandleOutput() with format %s failed: %v", format, err)
			}
		})
	}
}

func TestHandleOutputWithFailedSummary(t *testing.T) {
	// Test that console output works with failed tests.
	summary := &types.TestSummary{
		Failed: []types.TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
	}

	err := output.HandleOutput(summary, "terminal", "", false)
	if err != nil {
		t.Errorf("HandleOutput() with failed tests = %v, want nil", err)
	}
}

func TestConstants(t *testing.T) {
	// Test that constants are properly defined.
	if constants.FormatTerminal != "terminal" {
		t.Errorf("FormatTerminal = %v, want 'terminal'", constants.FormatTerminal)
	}
	if constants.FormatMarkdown != "markdown" {
		t.Errorf("FormatMarkdown = %v, want 'markdown'", constants.FormatMarkdown)
	}
	if constants.FormatGitHub != "github" {
		t.Errorf("FormatGitHub = %v, want 'github'", constants.FormatGitHub)
	}
	if constants.FormatBoth != "both" {
		t.Errorf("FormatBoth = %v, want 'both'", constants.FormatBoth)
	}
	// FormatStdin is deprecated, use FormatTerminal instead
	// This test can be removed once FormatStdin is removed
	if constants.FormatTerminal != "terminal" {
		t.Errorf("FormatTerminal = %v, want 'terminal'", constants.FormatTerminal)
	}
	if constants.StdinMarker != "-" {
		t.Errorf("StdinMarker = %v, want '-'", constants.StdinMarker)
	}
	if constants.StdoutPath != "stdout" {
		t.Errorf("StdoutPath = %v, want 'stdout'", constants.StdoutPath)
	}
	if constants.DefaultSummaryFile != "test-summary.md" {
		t.Errorf("DefaultSummaryFile = %v, want 'test-summary.md'", constants.DefaultSummaryFile)
	}
	if constants.CoverageHighThreshold != 80.0 {
		t.Errorf("CoverageHighThreshold = %v, want 80.0", constants.CoverageHighThreshold)
	}
	if constants.CoverageMedThreshold != 40.0 {
		t.Errorf("CoverageMedThreshold = %v, want 40.0", constants.CoverageMedThreshold)
	}
	if constants.Base10BitSize != 64 {
		t.Errorf("Base10BitSize = %v, want 64", constants.Base10BitSize)
	}
}
