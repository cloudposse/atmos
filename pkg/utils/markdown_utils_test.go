package utils

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

func TestPrintfMarkdown(t *testing.T) {
	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintfMarkdown("Atmos: %s", "Manage Environments Easily in Terraform")

	err := w.Close()
	assert.Nil(t, err)

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestPrintfMarkdown' should execute without error")

	// Check if output contains the expected content
	expectedOutput := "Atmos: Manage Environments Easily in Terraform"
	assert.Contains(t, output.String(), expectedOutput, "'TestPrintfMarkdown' output should contain information about Atmos")
}

func TestInitializeMarkdown(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	InitializeMarkdown(atmosConfig)
	assert.NotNil(t, render)
}

// TestPrintfMarkdownToTUI tests that markdown output goes to stderr.
func TestPrintfMarkdownToTUI(t *testing.T) {
	// Initialize the renderer
	InitializeMarkdown(schema.AtmosConfiguration{})

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Print to TUI (stderr)
	PrintfMarkdownToTUI("Test: %s", "Message to stderr")

	err := w.Close()
	require.NoError(t, err)

	os.Stderr = oldStderr

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	require.NoError(t, err)

	// Verify output went to stderr
	assert.Contains(t, output.String(), "Test: Message to stderr")
}

// TestMarkdownRendering tests various markdown elements.
func TestMarkdownRendering(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "Headers",
			input:    "# H1\n## H2\n### H3",
			contains: []string{"H1", "H2", "H3"},
		},
		{
			name:     "Bold text",
			input:    "This is **bold** text",
			contains: []string{"bold"},
		},
		{
			name:     "Italic text",
			input:    "This is *italic* text",
			contains: []string{"italic"},
		},
		{
			name:     "Code inline",
			input:    "Use `atmos` command",
			contains: []string{"atmos"},
		},
		{
			name:     "Links",
			input:    "Visit [Atmos](https://atmos.tools)",
			contains: []string{"Atmos", "https://atmos.tools"},
		},
		{
			name:     "Lists",
			input:    "- Item 1\n- Item 2\n- Item 3",
			contains: []string{"Item 1", "Item 2", "Item 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize renderer
			InitializeMarkdown(schema.AtmosConfiguration{})

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Print markdown
			PrintfMarkdown("%s", tt.input)

			w.Close()
			os.Stdout = oldStdout

			// Read output
			var output bytes.Buffer
			io.Copy(&output, r)
			result := output.String()

			// Verify all expected content is present
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected, "Expected '%s' in markdown output", expected)
			}
		})
	}
}

// TestMarkdownWithoutRenderer tests behavior when renderer is not initialized.
func TestMarkdownWithoutRenderer(t *testing.T) {
	// Set render to nil to simulate uninitialized state
	render = nil

	// Capture stdout
	oldStdout := os.Stdout
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	t.Cleanup(func() {
		r.Close()
	})

	os.Stdout = w

	// Should still print raw text without rendering
	PrintfMarkdown("## Test Header\n**Bold** text")

	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Read output
	var output bytes.Buffer
	if _, err := io.Copy(&output, r); err != nil {
		t.Fatalf("Failed to copy output: %v", err)
	}
	result := output.String()

	// Should contain raw markdown since renderer is nil
	assert.Contains(t, result, "## Test Header")
	assert.Contains(t, result, "**Bold**")
}

// TestTelemetryDisclosureMarkdown tests the telemetry disclosure message formatting.
func TestTelemetryDisclosureMarkdown(t *testing.T) {
	// Initialize renderer
	InitializeMarkdown(schema.AtmosConfiguration{})

	// Capture stderr (where TUI output goes)
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Print the telemetry disclosure message
	message := `**Notice:** Telemetry Enabled - Atmos now collects completely anonymous telemetry regarding usage. This information is used to shape the Atmos roadmap and prioritize features. You can learn more, including how to opt out if you'd not like to participate in this anonymous program, by visiting: https://atmos.tools/cli/telemetry`

	PrintfMarkdownToTUI("%s", message)

	w.Close()
	os.Stderr = oldStderr

	// Read output
	var output bytes.Buffer
	io.Copy(&output, r)
	result := output.String()

	// Verify key components are present
	assert.Contains(t, result, "Notice:")
	assert.Contains(t, result, "Telemetry Enabled")
	assert.Contains(t, result, "anonymous telemetry")
	assert.Contains(t, result, "https://atmos.tools/cli/telemetry")
}

// TestMarkdownFormatting tests specific formatting requirements.
func TestMarkdownFormatting(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []interface{}
		expect string
	}{
		{
			name:   "Simple string formatting",
			format: "Hello %s",
			args:   []interface{}{"World"},
			expect: "Hello World",
		},
		{
			name:   "Multiple arguments",
			format: "%s version %s",
			args:   []interface{}{"Atmos", "1.0.0"},
			expect: "Atmos version 1.0.0",
		},
		{
			name:   "Integer formatting",
			format: "Count: %d",
			args:   []interface{}{42},
			expect: "Count: 42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize renderer
			InitializeMarkdown(schema.AtmosConfiguration{})

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Print formatted markdown
			PrintfMarkdown(tt.format, tt.args...)

			w.Close()
			os.Stdout = oldStdout

			// Read output
			var output bytes.Buffer
			io.Copy(&output, r)
			result := strings.TrimSpace(output.String())

			// Verify formatting
			assert.Contains(t, result, tt.expect)
		})
	}
}
