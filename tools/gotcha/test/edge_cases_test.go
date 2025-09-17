package test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/gotcha/internal/markdown"
	"github.com/cloudposse/gotcha/internal/output"
	"github.com/cloudposse/gotcha/pkg/config"
	"github.com/cloudposse/gotcha/pkg/constants"
	"github.com/cloudposse/gotcha/pkg/types"
)

func TestWriteSummaryErrorHandling(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	// Test writing to invalid path.
	err := output.WriteSummary(summary, constants.FormatMarkdown, "/invalid/path/file.md")
	if err == nil {
		t.Error("WriteSummary() should return error for invalid path")
	}
}

// TestOpenOutputEdgeCases tests internal output opening logic
// Note: openOutput is not exported, so this test is commented out
// TODO: Consider making openOutput testable or testing through WriteSummary
/*
func TestOpenOutputEdgeCases(t *testing.T) {
	// Test creating file in existing directory.
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-summary-temp.md")

	// This would need openOutput to be exported or tested indirectly
}
*/

func TestWriteMarkdownContentWithGitHubActions(t *testing.T) {
	// Test with GITHUB_STEP_SUMMARY set.
	oldEnv := os.Getenv("GITHUB_STEP_SUMMARY")
	os.Setenv("GITHUB_STEP_SUMMARY", "/dev/null")
	defer func() {
		if oldEnv != "" {
			os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
		} else {
			os.Unsetenv("GITHUB_STEP_SUMMARY")
		}
	}()

	// Initialize viper to pick up the environment variables
	config.InitEnvironment()

	summary := &types.TestSummary{
		Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	var buf bytes.Buffer
	markdown.WriteContent(&buf, summary, constants.FormatGitHub)

	output := buf.String()

	// Should not include timestamp when GITHUB_STEP_SUMMARY is set.
	if strings.Contains(output, "_Generated:") {
		t.Error("WriteContent() should not include timestamp when GITHUB_STEP_SUMMARY is set")
	}

	// Should include test results header (may include emoji and discriminator).
	if !strings.Contains(output, "Test Results") {
		t.Error("WriteContent() missing test results header")
	}
}

// TestOpenGitHubOutputWithEnv tests GitHub output opening
// Note: openGitHubOutput is not exported, testing through WriteSummary instead.
func TestGitHubOutputWithEnv(t *testing.T) {
	// Create temporary file for test.
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-github-summary.md")
	file, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	file.Close()

	// Test with GITHUB_STEP_SUMMARY set.
	oldEnv := os.Getenv("GITHUB_STEP_SUMMARY")
	os.Setenv("GITHUB_STEP_SUMMARY", tempFile)
	defer func() {
		if oldEnv != "" {
			os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
		} else {
			os.Unsetenv("GITHUB_STEP_SUMMARY")
		}
	}()

	summary := &types.TestSummary{
		Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	// WriteSummary should write to GITHUB_STEP_SUMMARY when format is github
	err = output.WriteSummary(summary, constants.FormatGitHub, "")
	if err != nil {
		t.Errorf("WriteSummary() with GITHUB_STEP_SUMMARY error = %v", err)
	}
}

func TestHandleOutputBothFormat(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	// Test both format.
	err := output.HandleOutput(summary, "both", "-", true)
	if err != nil {
		t.Errorf("HandleOutput() both format = %v, want nil", err)
	}
}

func TestWriteSummaryToFile(t *testing.T) {
	summary := &types.TestSummary{
		Failed: []types.TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
	}

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-summary-output.md")
	err := output.WriteSummary(summary, constants.FormatMarkdown, tempFile)
	if err != nil {
		t.Errorf("WriteSummary() error = %v", err)
	}
}
