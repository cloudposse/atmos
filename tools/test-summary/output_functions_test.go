package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestWriteSummary(t *testing.T) {
	tests := []struct {
		name       string
		summary    *TestSummary
		format     string
		outputFile string
		wantError  bool
	}{
		{
			name: "markdown to stdout",
			summary: &TestSummary{
				Failed:   []TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.5}},
				Passed:   []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
				Coverage: "85.5%",
			},
			format:     formatMarkdown,
			outputFile: "-",
			wantError:  false,
		},
		{
			name: "github format without GITHUB_STEP_SUMMARY",
			summary: &TestSummary{
				Skipped: []TestResult{{Package: "test/pkg", Test: "TestSkip", Status: "skip", Duration: 0}},
			},
			format:     formatGitHub,
			outputFile: "",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeSummary(tt.summary, tt.format, tt.outputFile)
			if (err != nil) != tt.wantError {
				t.Errorf("writeSummary() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestHandleMarkdownOutput(t *testing.T) {
	summary := &TestSummary{
		Passed: []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
	}

	tests := []struct {
		name       string
		outputFile string
		wantError  bool
	}{
		{
			name:       "with output file",
			outputFile: "test-output.md",
			wantError:  false,
		},
		{
			name:       "empty output file defaults to stdout",
			outputFile: "",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleMarkdownOutput(summary, tt.outputFile)
			hasError := (err != nil)
			if hasError != tt.wantError {
				t.Errorf("handleMarkdownOutput() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSetupUsage(t *testing.T) {
	// Test that setupUsage function configures flag.Usage properly.
	// We can't easily capture the output, but we can test that the function runs without error.
	setupUsage()

	// Test that flag.Usage is not nil after setup.
	if flag.Usage == nil {
		t.Error("setupUsage() should set flag.Usage")
	}

	// We can verify the function was called by checking it's been assigned.
	// This is more of a smoke test to ensure the function doesn't panic.
}

func TestOpenOutput(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		outputFile string
		wantPath   string
		wantError  bool
	}{
		{
			name:       "markdown to stdout",
			format:     formatMarkdown,
			outputFile: "-",
			wantPath:   "stdout",
			wantError:  false,
		},
		{
			name:       "markdown empty defaults to stdout",
			format:     formatMarkdown,
			outputFile: "",
			wantPath:   "stdout",
			wantError:  false,
		},
		{
			name:       "github format",
			format:     formatGitHub,
			outputFile: "",
			wantPath:   defaultSummaryFile,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For github format tests, ensure we test local mode behavior by unsetting GITHUB_STEP_SUMMARY.
			if tt.format == formatGitHub {
				oldEnv, hasEnv := os.LookupEnv("GITHUB_STEP_SUMMARY")
				os.Unsetenv("GITHUB_STEP_SUMMARY")
				defer func() {
					if hasEnv {
						os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
					}
				}()
			}

			writer, path, err := openOutput(tt.format, tt.outputFile)
			if (err != nil) != tt.wantError {
				t.Errorf("openOutput() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if writer == nil {
				t.Error("openOutput() returned nil writer")
				return
			}
			if path != tt.wantPath && !strings.Contains(path, tt.wantPath) {
				t.Errorf("openOutput() path = %v, want %v", path, tt.wantPath)
			}

			// Close if it's a file.
			if closer, ok := writer.(io.Closer); ok && writer != os.Stdout {
				closer.Close()
			}
		})
	}
}

func TestOpenGitHubOutput(t *testing.T) {
	// Test without GITHUB_STEP_SUMMARY.
	t.Run("local mode", func(t *testing.T) {
		// Ensure we test local mode behavior by unsetting GITHUB_STEP_SUMMARY.
		oldEnv, hasEnv := os.LookupEnv("GITHUB_STEP_SUMMARY")
		os.Unsetenv("GITHUB_STEP_SUMMARY")
		defer func() {
			if hasEnv {
				os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
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
		if path != defaultSummaryFile {
			t.Errorf("openGitHubOutput() path = %v, want %v", path, defaultSummaryFile)
		}

		// Close and cleanup.
		if closer, ok := writer.(io.Closer); ok {
			closer.Close()
		}
		os.Remove(defaultSummaryFile)
	})

	// Test with custom output file.
	t.Run("custom output file", func(t *testing.T) {
		// Ensure we test local mode behavior by unsetting GITHUB_STEP_SUMMARY.
		oldEnv, hasEnv := os.LookupEnv("GITHUB_STEP_SUMMARY")
		os.Unsetenv("GITHUB_STEP_SUMMARY")
		defer func() {
			if hasEnv {
				os.Setenv("GITHUB_STEP_SUMMARY", oldEnv)
			}
		}()

		customFile := "custom-summary.md"
		writer, path, err := openGitHubOutput(customFile)
		if err != nil {
			t.Errorf("openGitHubOutput() error = %v", err)
			return
		}
		if writer == nil {
			t.Error("openGitHubOutput() returned nil writer")
			return
		}
		if path != customFile {
			t.Errorf("openGitHubOutput() path = %v, want %v", path, customFile)
		}

		// Close and cleanup.
		if closer, ok := writer.(io.Closer); ok {
			closer.Close()
		}
		os.Remove(customFile)
	})
}

func TestWriteMarkdownContent(t *testing.T) {
	tests := []struct {
		name    string
		summary *TestSummary
		format  string
		want    []string
	}{
		{
			name: "basic summary with all test types",
			summary: &TestSummary{
				Failed:   []TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.5}},
				Skipped:  []TestResult{{Package: "test/pkg", Test: "TestSkip", Status: "skip", Duration: 0}},
				Passed:   []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
				Coverage: "75.5%",
			},
			format: formatMarkdown,
			want: []string{
				"# Test Results",
				"[![Passed](https://shields.io/badge/PASSED-1-success?style=for-the-badge)](#user-content-passed)",
				"[![Failed](https://shields.io/badge/FAILED-1-critical?style=for-the-badge)](#user-content-failed)",
				"[![Skipped](https://shields.io/badge/SKIPPED-1-inactive?style=for-the-badge)](#user-content-skipped)",
				"### ❌ Failed Tests (1)",
				"### ⏭️ Skipped Tests (1)",
				"### ✅ Passed Tests (1)",
				"# Test Coverage",
				"| Statement Coverage | 75.5% | 🟡 |",
			},
		},
		{
			name: "github format with timestamp",
			summary: &TestSummary{
				Passed: []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
			},
			format: formatGitHub,
			want: []string{
				"_Generated:",
				"# Test Results",
			},
		},
		{
			name: "no coverage",
			summary: &TestSummary{
				Passed: []TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
			},
			format: formatMarkdown,
			want: []string{
				"# Test Results",
				"[![Passed](https://shields.io/badge/PASSED-1-success?style=for-the-badge)](#user-content-passed)",
				"[![Failed](https://shields.io/badge/FAILED-0-critical?style=for-the-badge)](#user-content-failed)",
				"[![Skipped](https://shields.io/badge/SKIPPED-0-inactive?style=for-the-badge)](#user-content-skipped)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Set up environment for timestamp test.
			if tt.format == formatGitHub {
				os.Unsetenv("GITHUB_STEP_SUMMARY")
			}

			writeMarkdownContent(&buf, tt.summary, tt.format)

			output := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("writeMarkdownContent() missing expected content: %s\nGot:\n%s", want, output)
				}
			}
		})
	}
}
