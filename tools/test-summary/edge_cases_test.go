package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSummaryErrorHandling(t *testing.T) {
	summary := &TestSummary{
		Passed: []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	// Test writing to invalid path
	err := writeSummary(summary, formatMarkdown, "/invalid/path/file.md")
	if err == nil {
		t.Error("writeSummary() should return error for invalid path")
	}
}

func TestOpenOutputEdgeCases(t *testing.T) {
	// Test creating file in existing directory
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-summary-temp.md")

	writer, path, err := openOutput(formatMarkdown, tempFile)
	if err != nil {
		t.Errorf("openOutput() error = %v", err)
		return
	}
	if writer == nil {
		t.Error("openOutput() returned nil writer")
		return
	}
	if path != tempFile {
		t.Errorf("openOutput() path = %v, want %v", path, tempFile)
	}

	// Clean up
	if closer, ok := writer.(io.Closer); ok {
		closer.Close()
	}
}

func TestWriteMarkdownContentWithGitHubActions(t *testing.T) {
	// Test with GITHUB_STEP_SUMMARY set
	oldEnv := os.Getenv("GITHUB_STEP_SUMMARY")
	os.Setenv("GITHUB_STEP_SUMMARY", "/dev/null")
	defer func() {
		if oldEnv != "" {
			os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
		} else {
			os.Unsetenv("GITHUB_STEP_SUMMARY")
		}
	}()

	summary := &TestSummary{
		Passed: []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	var buf bytes.Buffer
	writeMarkdownContent(&buf, summary, formatGitHub)

	output := buf.String()

	// Should not include timestamp when GITHUB_STEP_SUMMARY is set
	if strings.Contains(output, "_Generated:") {
		t.Error("writeMarkdownContent() should not include timestamp when GITHUB_STEP_SUMMARY is set")
	}

	// Should include test results
	if !strings.Contains(output, "## Test Results") {
		t.Error("writeMarkdownContent() missing test results header")
	}
}

func TestOpenGitHubOutputWithEnv(t *testing.T) {
	// Create temporary file for test
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-github-summary.md")
	file, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	file.Close()

	// Test with GITHUB_STEP_SUMMARY set
	oldEnv := os.Getenv("GITHUB_STEP_SUMMARY")
	os.Setenv("GITHUB_STEP_SUMMARY", tempFile)
	defer func() {
		if oldEnv != "" {
			os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
		} else {
			os.Unsetenv("GITHUB_STEP_SUMMARY")
		}
	}()

	writer, path, err := openGitHubOutput("")
	if err != nil {
		t.Errorf("openGitHubOutput() error = %v", err)
		return
	}
	if writer == nil {
		t.Error("openGitHubOutput() returned nil writer")
		return
	}
	if path != tempFile {
		t.Errorf("openGitHubOutput() path = %v, want %v", path, tempFile)
	}

	// Clean up
	if closer, ok := writer.(io.Closer); ok {
		closer.Close()
	}
}

func TestHandleOutputBothFormat(t *testing.T) {
	summary := &TestSummary{
		Passed:   []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
		ExitCode: 0,
	}
	consoleOutput := "test console output"

	// Capture output for both format
	exitCode := handleOutput(formatBoth, "-", summary, consoleOutput)

	if exitCode != 0 {
		t.Errorf("handleOutput() both format = %v, want 0", exitCode)
	}
}

func TestWriteSummaryToFile(t *testing.T) {
	summary := &TestSummary{
		Failed: []TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.0}},
	}

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test-summary-output.md")
	err := writeSummary(summary, formatMarkdown, tempFile)
	if err != nil {
		t.Errorf("writeSummary() error = %v", err)
	}
}
