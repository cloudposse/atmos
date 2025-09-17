package output

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/gotcha/internal/markdown"
	"github.com/cloudposse/gotcha/pkg/types"
)

func TestWriteCoverageSection(t *testing.T) {
	tests := []struct {
		name      string
		coverage  string
		wantEmoji string
		wantText  string
	}{
		{
			name:      "high coverage",
			coverage:  "85.5%",
			wantEmoji: "🟢",
			wantText:  "85.5%",
		},
		{
			name:      "medium coverage",
			coverage:  "65.0%",
			wantEmoji: "🟡",
			wantText:  "65.0%",
		},
		{
			name:      "low coverage",
			coverage:  "30.0%",
			wantEmoji: "🔴",
			wantText:  "30.0%",
		},
		{
			name:      "exact high threshold",
			coverage:  "80.0%",
			wantEmoji: "🟢",
			wantText:  "80.0%",
		},
		{
			name:      "exact medium threshold",
			coverage:  "40.0%",
			wantEmoji: "🟡",
			wantText:  "40.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// Use the markdown package function that actually writes coverage with emojis
			markdown.WriteBasicCoverage(&buf, tt.coverage)
			output := buf.String()

			checkContainsAll(t, output, tt.wantEmoji, tt.wantText, "Statement Coverage")
		})
	}
}

func TestShortPackage(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		want string
	}{
		{
			name: "full github path",
			pkg:  "github.com/cloudposse/atmos/cmd",
			want: "cmd",
		},
		{
			name: "simple path",
			pkg:  "pkg/utils",
			want: "utils",
		},
		{
			name: "single component",
			pkg:  "main",
			want: "main",
		},
		{
			name: "empty string",
			pkg:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: shortPackage is now in types package
			got := types.ShortPackage(tt.pkg)
			if got != tt.want {
				t.Errorf("shortPackage(%q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

// checkContainsAll checks if the output contains all expected strings.
func checkContainsAll(t *testing.T, got string, want ...string) {
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("Output missing expected content: %s\nGot:\n%s", w, got)
		}
	}
}

func TestHandleOutputWithGenerateSummary(t *testing.T) {
	// Create a test summary
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/test", Test: "TestPass", Duration: 1.0},
		},
		Failed:  []types.TestResult{},
		Skipped: []types.TestResult{},
	}

	tests := []struct {
		name            string
		format          string
		generateSummary bool
		wantFile        bool
	}{
		{
			name:            "markdown with generate-summary true",
			format:          "markdown",
			generateSummary: true,
			wantFile:        true,
		},
		{
			name:            "markdown with generate-summary false",
			format:          "markdown",
			generateSummary: false,
			wantFile:        false,
		},
		{
			name:            "github with generate-summary true",
			format:          "github",
			generateSummary: true,
			wantFile:        true,
		},
		{
			name:            "github with generate-summary false",
			format:          "github",
			generateSummary: false,
			wantFile:        false,
		},
		{
			name:            "terminal format ignores generate-summary",
			format:          "terminal",
			generateSummary: true,
			wantFile:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tempDir := t.TempDir()
			outputFile := filepath.Join(tempDir, "test-output.md")

			// Only provide output file when we expect a file to be created
			var testOutputFile string
			if tt.wantFile {
				testOutputFile = outputFile
			}

			// Run HandleOutput
			err := HandleOutput(summary, tt.format, testOutputFile, tt.generateSummary)
			if err != nil {
				t.Fatalf("HandleOutput failed: %v", err)
			}

			// Check if file was created as expected
			_, statErr := os.Stat(outputFile)
			fileExists := statErr == nil

			if tt.wantFile && !fileExists {
				t.Errorf("Expected file to be created but it wasn't")
			}
			if !tt.wantFile && fileExists {
				t.Errorf("Expected no file to be created but file exists")
			}
		})
	}
}

func TestDefaultOutputPaths(t *testing.T) {
	// Test that when no output file is specified but generate-summary is true,
	// the file is created as test-summary.md in the current directory
	t.Run("default test-summary.md location", func(t *testing.T) {
		// Create a test summary
		summary := &types.TestSummary{
			Passed: []types.TestResult{
				{Package: "pkg/test", Test: "TestPass", Duration: 1.0},
			},
		}

		// Change to temp directory for test
		tempDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		os.Chdir(tempDir)

		// Run HandleOutput with no output file specified
		err := HandleOutput(summary, "markdown", "", true)
		if err != nil {
			t.Fatalf("HandleOutput failed: %v", err)
		}

		// Check that test-summary.md was created in current directory
		expectedFile := filepath.Join(tempDir, "test-summary.md")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected test-summary.md to be created in current directory")
		}
	})
}
