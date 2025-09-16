package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGitHubSummaryNoDuplication(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Set up a fake GITHUB_STEP_SUMMARY file
	stepSummaryFile := filepath.Join(tempDir, "step_summary.md")
	// Create the file first (GitHub Actions creates this file)
	os.WriteFile(stepSummaryFile, []byte(""), 0o644)
	os.Setenv("GITHUB_STEP_SUMMARY", stepSummaryFile)
	defer os.Unsetenv("GITHUB_STEP_SUMMARY")

	// Set a unique UUID for the test
	os.Setenv("GOTCHA_COMMENT_UUID", "test-uuid-123")
	defer os.Unsetenv("GOTCHA_COMMENT_UUID")

	// Create test summary
	summary := &types.TestSummary{
		Passed:  []types.TestResult{{Test: "TestPass", Package: "test/pkg", Status: "pass"}},
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
	}

	// Write the summary (should only write to GITHUB_STEP_SUMMARY in CI)
	err := output.WriteSummary(summary, constants.FormatGitHub, "")
	assert.NoError(t, err)

	// Read the step summary file
	content, err := os.ReadFile(stepSummaryFile)
	assert.NoError(t, err)

	// Count occurrences of the UUID marker
	uuidCount := strings.Count(string(content), "test-uuid-123")
	assert.Equal(t, 1, uuidCount, "UUID should appear exactly once in the output")

	// Verify no duplicate test summary file was created
	_, err = os.Stat("test-summary.md")
	assert.True(t, os.IsNotExist(err), "test-summary.md should not be created when GITHUB_STEP_SUMMARY is set")
}

func TestGitHubSummaryLocalMode(t *testing.T) {
	// Ensure GITHUB_STEP_SUMMARY is not set (local mode)
	os.Unsetenv("GITHUB_STEP_SUMMARY")

	// Create a temporary directory for test files
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create test summary
	summary := &types.TestSummary{
		Passed:  []types.TestResult{{Test: "TestPass", Package: "test/pkg", Status: "pass"}},
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
	}

	// Write the summary (should write to test-summary.md in local mode)
	err := output.WriteSummary(summary, constants.FormatGitHub, "")
	assert.NoError(t, err)

	// Verify test-summary.md was created
	_, err = os.Stat("test-summary.md")
	assert.NoError(t, err, "test-summary.md should be created in local mode")

	// Read and verify content
	content, err := os.ReadFile("test-summary.md")
	assert.NoError(t, err)
	assert.Contains(t, string(content), "Test Results")
	assert.Contains(t, string(content), "TestPass")
}

func TestGitHubSummaryTruncatesFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Set up a fake GITHUB_STEP_SUMMARY file with existing content
	stepSummaryFile := filepath.Join(tempDir, "step_summary.md")
	// Create the file with old content
	os.WriteFile(stepSummaryFile, []byte("OLD CONTENT THAT SHOULD BE REPLACED\n"), 0o644)

	os.Setenv("GITHUB_STEP_SUMMARY", stepSummaryFile)
	defer os.Unsetenv("GITHUB_STEP_SUMMARY")

	// Create test summary
	summary := &types.TestSummary{
		Passed:  []types.TestResult{{Test: "TestNew", Package: "test/pkg", Status: "pass"}},
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
	}

	// Write the summary
	err := output.WriteSummary(summary, constants.FormatGitHub, "")
	assert.NoError(t, err)

	// Read the step summary file
	content, err := os.ReadFile(stepSummaryFile)
	assert.NoError(t, err)

	// Verify old content is gone and new content is present
	assert.NotContains(t, string(content), "OLD CONTENT", "Old content should be truncated")
	assert.Contains(t, string(content), "TestNew", "New content should be present")
	assert.Contains(t, string(content), "Test Results", "New header should be present")
}
