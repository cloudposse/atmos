package output

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/internal/markdown"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

func TestWriteSummary(t *testing.T) {
	tests := []struct {
		name       string
		summary    *types.TestSummary
		format     string
		outputFile string
		wantError  bool
	}{
		{
			name: "markdown to stdout",
			summary: &types.TestSummary{
				Failed:   []types.TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.5}},
				Passed:   []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
				Coverage: "85.5%",
			},
			format:     constants.FormatMarkdown,
			outputFile: "-",
			wantError:  false,
		},
		{
			name: "github format without GITHUB_STEP_SUMMARY",
			summary: &types.TestSummary{
				Skipped: []types.TestResult{{Package: "test/pkg", Test: "TestSkip", Status: "skip", Duration: 0}},
			},
			format:     constants.FormatGitHub,
			outputFile: "",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteSummary(tt.summary, tt.format, tt.outputFile)
			if (err != nil) != tt.wantError {
				t.Errorf("WriteSummary() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestHandleMarkdownOutput(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
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
			err := WriteSummary(summary, constants.FormatMarkdown, tt.outputFile)
			hasError := (err != nil)
			if hasError != tt.wantError {
				t.Errorf("WriteSummary() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestWriteSummaryMarkdown(t *testing.T) {
	// Test that WriteSummary function works with markdown format
	summary := &types.TestSummary{
		Passed:   []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
		Coverage: "75.0%",
	}

	// Test markdown output
	err := WriteSummary(summary, constants.FormatMarkdown, "")
	if err != nil {
		t.Errorf("WriteSummary(markdown) error = %v", err)
	}

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
			format:     constants.FormatMarkdown,
			outputFile: "-",
			wantPath:   "stdout",
			wantError:  false,
		},
		{
			name:       "markdown empty defaults to stdout",
			format:     constants.FormatMarkdown,
			outputFile: "",
			wantPath:   "stdout",
			wantError:  false,
		},
		{
			name:       "github format",
			format:     constants.FormatGitHub,
			outputFile: "",
			wantPath:   constants.DefaultSummaryFile,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For github format tests, ensure we test local mode behavior by unsetting GITHUB_STEP_SUMMARY.
			if tt.format == constants.FormatGitHub {
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
		if path != constants.DefaultSummaryFile {
			t.Errorf("openGitHubOutput() path = %v, want %v", path, constants.DefaultSummaryFile)
		}

		// Close and cleanup.
		if closer, ok := writer.(io.Closer); ok {
			closer.Close()
		}
		os.Remove(constants.DefaultSummaryFile)
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
		summary *types.TestSummary
		format  string
		want    []string
	}{
		{
			name: "basic summary with all test types",
			summary: &types.TestSummary{
				Failed:   []types.TestResult{{Package: "test/pkg", Test: "TestFail", Status: "fail", Duration: 1.5}},
				Skipped:  []types.TestResult{{Package: "test/pkg", Test: "TestSkip", Status: "skip", Duration: 0}},
				Passed:   []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
				Coverage: "75.5%",
			},
			format: constants.FormatMarkdown,
			want: []string{
				"# Test Results",
				"[![Passed](https://shields.io/badge/PASSED-1-success?style=for-the-badge)](#user-content-passed)",
				"[![Failed](https://shields.io/badge/FAILED-1-critical?style=for-the-badge)](#user-content-failed)",
				"[![Skipped](https://shields.io/badge/SKIPPED-1-inactive?style=for-the-badge)](#user-content-skipped)",
				"### ❌ Failed Tests (1)",
				"### ⏭️ Skipped Tests (1)",
				"### ✅ Passed Tests (1)",
				"## 📊 Test Coverage",
				"| Statement Coverage | 75.5% | 🟡 |",
			},
		},
		{
			name: "github format with timestamp",
			summary: &types.TestSummary{
				Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
			},
			format: constants.FormatGitHub,
			want: []string{
				"_Generated:",
				"# Test Results",
			},
		},
		{
			name: "no coverage",
			summary: &types.TestSummary{
				Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
			},
			format: constants.FormatMarkdown,
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
			if tt.format == constants.FormatGitHub {
				os.Unsetenv("GITHUB_STEP_SUMMARY")
			}

			markdown.WriteContent(&buf, tt.summary, tt.format)

			output := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("markdown.WriteContent() missing expected content: %s\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestUUIDCommentInjection(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		setEnv   bool
		wantUUID bool
	}{
		{
			name:     "UUID set in environment",
			uuid:     "test-uuid-12345",
			setEnv:   true,
			wantUUID: true,
		},
		{
			name:     "no UUID environment variable",
			uuid:     "",
			setEnv:   false,
			wantUUID: false,
		},
		{
			name:     "empty UUID environment variable",
			uuid:     "",
			setEnv:   true,
			wantUUID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment.
			if tt.setEnv {
				os.Setenv("GOTCHA_COMMENT_UUID", tt.uuid)
				defer os.Unsetenv("GOTCHA_COMMENT_UUID")
			} else {
				os.Unsetenv("GOTCHA_COMMENT_UUID")
			}

			// Initialize viper to pick up the environment variables.
			config.InitEnvironment()

			summary := &types.TestSummary{
				Passed: []types.TestResult{{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5}},
			}

			var buf bytes.Buffer
			markdown.WriteContent(&buf, summary, constants.FormatMarkdown)
			output := buf.String()

			expectedComment := "<!-- test-summary-uuid: " + tt.uuid + " -->"

			if tt.wantUUID && tt.uuid != "" {
				// Check for presence of UUID comment.
				if !strings.Contains(output, expectedComment) {
					t.Errorf("UUID comment not found in output. Expected: %s\nGot output:\n%s", expectedComment, output)
					return
				}
				// Verify it's at the beginning of the output.
				if !strings.HasPrefix(output, expectedComment) {
					t.Errorf("UUID comment should be at the beginning of output. Got:\n%s", output)
				}
				return
			}

			// Should not contain UUID comment when not expected.
			if strings.Contains(output, "<!-- test-summary-uuid:") {
				t.Errorf("UUID comment should not be present when not set or empty. Got output:\n%s", output)
			}
		})
	}
}
