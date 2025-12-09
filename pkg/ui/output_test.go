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
		name    string
		text    string
		want    string
		wantErr bool
	}{
		{
			name:    "simple text",
			text:    "hello world",
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "empty string",
			text:    "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "text with newline",
			text:    "line1\nline2",
			want:    "line1\nline2",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Write(tt.text)

			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

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
		name    string
		format  string
		args    []interface{}
		want    string
		wantErr bool
	}{
		{
			name:    "simple format",
			format:  "hello %s",
			args:    []interface{}{"world"},
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "multiple arguments",
			format:  "count=%d, name=%s",
			args:    []interface{}{42, "test"},
			want:    "count=42, name=test",
			wantErr: false,
		},
		{
			name:    "no arguments",
			format:  "static text",
			args:    []interface{}{},
			want:    "static text",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Writef(tt.format, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("Writef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

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
		name    string
		text    string
		want    string
		wantErr bool
	}{
		{
			name:    "simple text with newline",
			text:    "hello world",
			want:    "hello world\n",
			wantErr: false,
		},
		{
			name:    "empty string with newline",
			text:    "",
			want:    "\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Writeln(tt.text)

			if (err != nil) != tt.wantErr {
				t.Errorf("Writeln() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

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

	err := Success("Deployment complete")
	if err != nil {
		t.Errorf("Success() error = %v", err)
	}

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

	err := Successf("Deployed %d components", 42)
	if err != nil {
		t.Errorf("Successf() error = %v", err)
	}

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

	err := Error("Configuration failed")
	if err != nil {
		t.Errorf("Error() error = %v", err)
	}

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

	err := Errorf("Failed to process %s", "component")
	if err != nil {
		t.Errorf("Errorf() error = %v", err)
	}

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

	err := Warning("Stack is deprecated")
	if err != nil {
		t.Errorf("Warning() error = %v", err)
	}

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

	err := Warningf("Deprecated in version %s", "2.0")
	if err != nil {
		t.Errorf("Warningf() error = %v", err)
	}

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

	err := Info("Processing components")
	if err != nil {
		t.Errorf("Info() error = %v", err)
	}

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

	err := Infof("Processing %d/%d components", 10, 100)
	if err != nil {
		t.Errorf("Infof() error = %v", err)
	}

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

	err := Markdown("# Test\n\nContent")
	if err != nil {
		t.Errorf("Markdown() error = %v", err)
	}

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

	err := Markdownf("# %s\n\n%s", "Title", "Content")
	if err != nil {
		t.Errorf("Markdownf() error = %v", err)
	}

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

	err := MarkdownMessage("**Error:** Invalid config")
	if err != nil {
		t.Errorf("MarkdownMessage() error = %v", err)
	}

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

	err := MarkdownMessagef("**%s:** %s", "Error", "Invalid config")
	if err != nil {
		t.Errorf("MarkdownMessagef() error = %v", err)
	}

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

	// Test that all package-level functions return errors when not initialized.
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Success", func() error { return Success("test") }},
		{"Successf", func() error { return Successf("test %s", "arg") }},
		{"Error", func() error { return Error("test") }},
		{"Errorf", func() error { return Errorf("test %s", "arg") }},
		{"Warning", func() error { return Warning("test") }},
		{"Warningf", func() error { return Warningf("test %s", "arg") }},
		{"Info", func() error { return Info("test") }},
		{"Infof", func() error { return Infof("test %s", "arg") }},
		{"Write", func() error { return Write("test") }},
		{"Writef", func() error { return Writef("test %s", "arg") }},
		{"Writeln", func() error { return Writeln("test") }},
		{"Markdown", func() error { return Markdown("# test") }},
		{"Markdownf", func() error { return Markdownf("# %s", "test") }},
		{"MarkdownMessage", func() error { return MarkdownMessage("**test**") }},
		{"MarkdownMessagef", func() error { return MarkdownMessagef("**%s**", "test") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Errorf("%s() should return error when formatter not initialized", tt.name)
			}
		})
	}
}

func TestMarkdown_ErrorPath(t *testing.T) {
	stdout, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	// Test with very large content that might cause rendering issues.
	// The glamour renderer handles this gracefully.
	largeContent := strings.Repeat("# Header\n\nContent\n\n", 1000)

	err := Markdown(largeContent)
	// Should not error even with large content.
	if err != nil {
		t.Errorf("Markdown() error = %v", err)
	}

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

	err := MarkdownMessage(largeContent)
	// Should not error even with large content.
	if err != nil {
		t.Errorf("MarkdownMessage() error = %v", err)
	}

	// Should have output to stderr.
	if stderr.Len() == 0 {
		t.Error("MarkdownMessage() did not write to stderr")
	}

	// Should not write to stdout.
	if stdout.Len() != 0 {
		t.Errorf("MarkdownMessage() wrote to stdout: %q", stdout.String())
	}
}
