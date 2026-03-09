package ui

import (
	"bytes"
	stdio "io"
	"strings"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// testStreams is a simple streams implementation for testing.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// setupTestUI creates test I/O context and initializes UI formatter.
func setupTestUI(t *testing.T) (stdout, stderr *bytes.Buffer, cleanup func()) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &testStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to create I/O context: %v", err)
	}

	// Save old formatter.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldIO := globalIO
	formatterMu.Unlock()

	// Initialize UI formatter.
	InitFormatter(ioCtx)

	// Note: We don't override globalFormatter here because InitFormatter already set it up correctly.
	// The formatter will use ioCtx which writes to our test buffers through the mocked streams.

	// Return cleanup function to restore old formatter.
	cleanup = func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		globalIO = oldIO
		formatterMu.Unlock()
	}

	return stdout, stderr, cleanup
}

func TestInitFormatter(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	streams := &testStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to create I/O context: %v", err)
	}

	InitFormatter(ioCtx)

	// Verify formatter was initialized.
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalFormatter == nil {
		t.Error("InitFormatter() did not initialize globalFormatter")
	}

	if globalIO == nil {
		t.Error("InitFormatter() did not initialize globalIO")
	}
}

func TestWrite(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "simple text",
			text: "hello world",
			want: "hello world",
		},
		{
			name: "empty string",
			text: "",
			want: "",
		},
		{
			name: "text with newline",
			text: "line1\nline2",
			want: "line1\nline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			// Write no longer returns an error.
			Write(tt.text)

			// Verify output went to stderr (UI channel).
			got := stderr.String()
			if got != tt.want {
				t.Errorf("Write() stderr = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stdout.
			if stdout.Len() != 0 {
				t.Errorf("Write() wrote to stdout: %q", stdout.String())
			}
		})
	}
}

func TestWritef(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	tests := []struct {
		name   string
		format string
		args   []interface{}
		want   string
	}{
		{
			name:   "simple format",
			format: "hello %s",
			args:   []interface{}{"world"},
			want:   "hello world",
		},
		{
			name:   "multiple arguments",
			format: "count=%d, name=%s",
			args:   []interface{}{42, "test"},
			want:   "count=42, name=test",
		},
		{
			name:   "no arguments",
			format: "static text",
			args:   []interface{}{},
			want:   "static text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			// Writef no longer returns an error.
			Writef(tt.format, tt.args...)

			got := stderr.String()
			if got != tt.want {
				t.Errorf("Writef() stderr = %q, want %q", got, tt.want)
			}

			if stdout.Len() != 0 {
				t.Errorf("Writef() wrote to stdout: %q", stdout.String())
			}
		})
	}
}

func TestWriteln(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "simple text with newline",
			text: "hello world",
			want: "hello world\n",
		},
		{
			name: "empty string with newline",
			text: "",
			want: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			// Writeln no longer returns an error.
			Writeln(tt.text)

			got := stderr.String()
			if got != tt.want {
				t.Errorf("Writeln() stderr = %q, want %q", got, tt.want)
			}

			if stdout.Len() != 0 {
				t.Errorf("Writeln() wrote to stdout: %q", stdout.String())
			}
		})
	}
}

func TestSuccess(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Success no longer returns an error.
	Success("Deployment complete")

	// Verify output went to stderr.
	output := stderr.String()
	if !strings.Contains(output, "Deployment complete") {
		t.Errorf("Success() output does not contain expected text, got: %q", output)
	}

	// Should end with newline.
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Success() output should end with newline, got: %q", output)
	}

	// Verify nothing written to stdout.
	if stdout.Len() != 0 {
		t.Errorf("Success() wrote to stdout: %q", stdout.String())
	}
}

func TestSuccessf(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Successf no longer returns an error.
	Successf("Deployed %d components", 42)

	output := stderr.String()
	if !strings.Contains(output, "Deployed 42 components") {
		t.Errorf("Successf() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Successf() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Successf() wrote to stdout: %q", stdout.String())
	}
}

func TestError(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Error no longer returns an error.
	Error("Configuration failed")

	output := stderr.String()
	if !strings.Contains(output, "Configuration failed") {
		t.Errorf("Error() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Error() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Error() wrote to stdout: %q", stdout.String())
	}
}

func TestErrorf(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Errorf no longer returns an error.
	Errorf("Failed to process %s", "component")

	output := stderr.String()
	if !strings.Contains(output, "Failed to process component") {
		t.Errorf("Errorf() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Errorf() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Errorf() wrote to stdout: %q", stdout.String())
	}
}

func TestWarning(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Warning no longer returns an error.
	Warning("Stack is deprecated")

	output := stderr.String()
	if !strings.Contains(output, "Stack is deprecated") {
		t.Errorf("Warning() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Warning() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Warning() wrote to stdout: %q", stdout.String())
	}
}

func TestWarningf(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Warningf no longer returns an error.
	Warningf("Deprecated in version %s", "2.0")

	output := stderr.String()
	if !strings.Contains(output, "Deprecated in version 2.0") {
		t.Errorf("Warningf() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Warningf() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Warningf() wrote to stdout: %q", stdout.String())
	}
}

func TestInfo(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Info no longer returns an error.
	Info("Processing components")

	output := stderr.String()
	if !strings.Contains(output, "Processing components") {
		t.Errorf("Info() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Info() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Info() wrote to stdout: %q", stdout.String())
	}
}

func TestInfof(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Infof no longer returns an error.
	Infof("Processing %d/%d components", 10, 100)

	output := stderr.String()
	if !strings.Contains(output, "Processing 10/100 components") {
		t.Errorf("Infof() output does not contain expected text, got: %q", output)
	}

	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Infof() output should end with newline, got: %q", output)
	}

	if stdout.Len() != 0 {
		t.Errorf("Infof() wrote to stdout: %q", stdout.String())
	}
}

func TestMarkdown(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Markdown no longer returns an error.
	Markdown("# Test\n\nContent")

	// Markdown goes to stdout (data channel).
	output := stdout.String()
	if len(output) == 0 {
		t.Error("Markdown() did not write to stdout")
	}

	// Verify nothing written to stderr.
	if stderr.Len() != 0 {
		t.Errorf("Markdown() wrote to stderr: %q", stderr.String())
	}
}

func TestMarkdownf(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Markdownf no longer returns an error.
	Markdownf("# %s\n\n%s", "Title", "Content")

	output := stdout.String()
	if len(output) == 0 {
		t.Error("Markdownf() did not write to stdout")
	}

	if stderr.Len() != 0 {
		t.Errorf("Markdownf() wrote to stderr: %q", stderr.String())
	}
}

func TestMarkdownMessage(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// MarkdownMessage no longer returns an error.
	MarkdownMessage("**Error:** Invalid config")

	// MarkdownMessage goes to stderr (UI channel).
	output := stderr.String()
	if len(output) == 0 {
		t.Error("MarkdownMessage() did not write to stderr")
	}

	// Verify nothing written to stdout.
	if stdout.Len() != 0 {
		t.Errorf("MarkdownMessage() wrote to stdout: %q", stdout.String())
	}
}

func TestMarkdownMessagef(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// MarkdownMessagef no longer returns an error.
	MarkdownMessagef("**%s:** %s", "Error", "Invalid config")

	output := stderr.String()
	if len(output) == 0 {
		t.Error("MarkdownMessagef() did not write to stderr")
	}

	if stdout.Len() != 0 {
		t.Errorf("MarkdownMessagef() wrote to stdout: %q", stdout.String())
	}
}

func TestGetFormatter_NotInitialized(t *testing.T) {
	// Save current formatter.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	globalFormatter = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		formatterMu.Unlock()
	}()

	// Should return error when not initialized.
	_, err := getFormatter()
	if err == nil {
		t.Error("getFormatter() should return error when not initialized")
	}
}

func TestPackageFunctions_NotInitialized(t *testing.T) {
	// Save current globals.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldTerminal := globalTerminal
	oldFormat := Format
	oldIO := globalIO
	globalFormatter = nil
	globalTerminal = nil
	Format = nil
	globalIO = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		globalTerminal = oldTerminal
		Format = oldFormat
		globalIO = oldIO
		formatterMu.Unlock()
	}()

	// Test that all package-level functions don't panic when not initialized.
	// Functions no longer return errors - they log internally and return gracefully.
	tests := []struct {
		name string
		fn   func()
	}{
		{"Success", func() { Success("test") }},
		{"Successf", func() { Successf("test %s", "arg") }},
		{"Error", func() { Error("test") }},
		{"Errorf", func() { Errorf("test %s", "arg") }},
		{"Warning", func() { Warning("test") }},
		{"Warningf", func() { Warningf("test %s", "arg") }},
		{"Info", func() { Info("test") }},
		{"Infof", func() { Infof("test %s", "arg") }},
		{"Write", func() { Write("test") }},
		{"Writef", func() { Writef("test %s", "arg") }},
		{"Writeln", func() { Writeln("test") }},
		{"Markdown", func() { Markdown("# test") }},
		{"Markdownf", func() { Markdownf("# %s", "test") }},
		{"MarkdownMessage", func() { MarkdownMessage("**test**") }},
		{"MarkdownMessagef", func() { MarkdownMessagef("**%s**", "test") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic - functions no longer return errors.
			tt.fn()
		})
	}
}

func TestMarkdown_ErrorPath(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Test with very large content that might cause rendering issues.
	// The glamour renderer handles this gracefully.
	largeContent := strings.Repeat("# Header\n\nContent\n\n", 1000)

	// Markdown no longer returns an error.
	Markdown(largeContent)

	// Should have output to stdout.
	if stdout.Len() == 0 {
		t.Error("Markdown() did not write to stdout")
	}

	// Should not write to stderr.
	if stderr.Len() != 0 {
		t.Errorf("Markdown() wrote to stderr: %q", stderr.String())
	}
}

func TestMarkdownMessage_ErrorPath(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Test with very large content.
	largeContent := strings.Repeat("**Error:** "+strings.Repeat("x", 100)+"\n\n", 100)

	// MarkdownMessage no longer returns an error.
	MarkdownMessage(largeContent)

	// Should have output to stderr.
	if stderr.Len() == 0 {
		t.Error("MarkdownMessage() did not write to stderr")
	}

	// Should not write to stdout.
	if stdout.Len() != 0 {
		t.Errorf("MarkdownMessage() wrote to stdout: %q", stdout.String())
	}
}
