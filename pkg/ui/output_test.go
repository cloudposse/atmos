package ui

import (
	"bytes"
	stdIo "io"
	"strings"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestNewOutput(t *testing.T) {
	ioCtx := createTestIOContext()

	out := NewOutput(ioCtx)

	if out == nil {
		t.Fatal("NewOutput() returned nil")
	}

	if out.Formatter() == nil {
		t.Error("Formatter() returned nil")
	}

	if out.IOContext() == nil {
		t.Error("IOContext() returned nil")
	}
}

func TestNewOutput_WithOptions(t *testing.T) {
	ioCtx := createTestIOContext()

	out := NewOutput(ioCtx, WithTrimTrailingWhitespace(true))

	if !out.TrimTrailingWhitespace() {
		t.Error("WithTrimTrailingWhitespace option not applied")
	}
}

func TestOutput_Print(t *testing.T) {
	ioCtx, stdout, _ := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Print("hello")

	got := stdout.String()
	if got != "hello" {
		t.Errorf("Print() = %q, want %q", got, "hello")
	}
}

func TestOutput_Printf(t *testing.T) {
	ioCtx, stdout, _ := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Printf("hello %s", "world")

	got := stdout.String()
	if got != "hello world" {
		t.Errorf("Printf() = %q, want %q", got, "hello world")
	}
}

func TestOutput_Println(t *testing.T) {
	ioCtx, stdout, _ := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Println("hello")

	got := stdout.String()
	if got != "hello\n" {
		t.Errorf("Println() = %q, want %q", got, "hello\n")
	}
}

func TestOutput_Message(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Message("test message")

	got := stderr.String()
	if !strings.Contains(got, "test message") {
		t.Errorf("Message() output %q doesn't contain 'test message'", got)
	}
}

func TestOutput_Success(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Success("operation succeeded")

	got := stderr.String()
	if !strings.Contains(got, "operation succeeded") {
		t.Errorf("Success() output %q doesn't contain 'operation succeeded'", got)
	}
}

func TestOutput_Warning(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Warning("warning message")

	got := stderr.String()
	if !strings.Contains(got, "warning message") {
		t.Errorf("Warning() output %q doesn't contain 'warning message'", got)
	}
}

func TestOutput_Error(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Error("error occurred")

	got := stderr.String()
	if !strings.Contains(got, "error occurred") {
		t.Errorf("Error() output %q doesn't contain 'error occurred'", got)
	}
}

func TestOutput_Info(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	out.Info("info message")

	got := stderr.String()
	if !strings.Contains(got, "info message") {
		t.Errorf("Info() output %q doesn't contain 'info message'", got)
	}
}

func TestOutput_Markdown(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple markdown",
			input:   "# Title\n\nParagraph",
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx, stdout, _ := createTestIOContextWithBuffers()
			out := NewOutput(ioCtx)

			err := out.Markdown(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Markdown() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && stdout.Len() == 0 && tt.input != "" {
				t.Error("Markdown() produced no output")
			}
		})
	}
}

func TestOutput_MarkdownUI(t *testing.T) {
	ioCtx, _, stderr := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	err := out.MarkdownUI("# Title")
	if err != nil {
		t.Errorf("MarkdownUI() error = %v", err)
	}

	if stderr.Len() == 0 {
		t.Error("MarkdownUI() produced no output")
	}
}

func TestOutput_TrimTrailingWhitespace(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		input   string
		want    string
	}{
		{
			name:    "disabled - no trimming",
			enabled: false,
			input:   "hello   ",
			want:    "hello   ",
		},
		{
			name:    "enabled - trim spaces",
			enabled: true,
			input:   "hello   ",
			want:    "hello",
		},
		{
			name:    "enabled - trim tabs",
			enabled: true,
			input:   "hello\t\t",
			want:    "hello",
		},
		{
			name:    "enabled - multiple lines",
			enabled: true,
			input:   "line1   \nline2\t\nline3",
			want:    "line1\nline2\nline3",
		},
		{
			name:    "enabled - preserve newlines",
			enabled: true,
			input:   "hello   \n",
			want:    "hello\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx, stdout, _ := createTestIOContextWithBuffers()
			out := NewOutput(ioCtx, WithTrimTrailingWhitespace(tt.enabled))

			out.Print(tt.input)

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Print() with trim=%v = %q, want %q", tt.enabled, got, tt.want)
			}
		})
	}
}

func TestOutput_SetTrimTrailingWhitespace(t *testing.T) {
	ioCtx, stdout, _ := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	// Initial state
	if out.TrimTrailingWhitespace() {
		t.Error("expected TrimTrailingWhitespace to be false initially")
	}

	// Enable trimming
	out.SetTrimTrailingWhitespace(true)
	if !out.TrimTrailingWhitespace() {
		t.Error("expected TrimTrailingWhitespace to be true after setting")
	}

	// Test that it actually trims
	out.Print("test   ")
	got := stdout.String()
	if got != "test" {
		t.Errorf("Print() with trimming = %q, want %q", got, "test")
	}

	// Disable trimming
	stdout.Reset()
	out.SetTrimTrailingWhitespace(false)
	out.Print("test   ")
	got = stdout.String()
	if got != "test   " {
		t.Errorf("Print() without trimming = %q, want %q", got, "test   ")
	}
}

func TestTrimTrailingSpaces(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no trailing spaces",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "trailing spaces",
			input: "hello world   ",
			want:  "hello world",
		},
		{
			name:  "trailing tabs",
			input: "hello world\t\t",
			want:  "hello world",
		},
		{
			name:  "mixed trailing whitespace",
			input: "hello world \t ",
			want:  "hello world",
		},
		{
			name:  "multiple lines",
			input: "line1   \nline2\t\nline3  ",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "empty lines",
			input: "   \n\t\n  ",
			want:  "\n\n",
		},
		{
			name:  "preserve internal spaces",
			input: "hello   world   ",
			want:  "hello   world",
		},
		{
			name:  "newline at end",
			input: "hello   \n",
			want:  "hello\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimTrailingSpaces(tt.input)
			if got != tt.want {
				t.Errorf("trimTrailingSpaces() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutput_ProcessOutput(t *testing.T) {
	ioCtx, stdout, _ := createTestIOContextWithBuffers()
	out := NewOutput(ioCtx)

	// Test all output methods respect trimming
	out.SetTrimTrailingWhitespace(true)

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "Print",
			fn:   func() { out.Print("test   ") },
		},
		{
			name: "Printf",
			fn:   func() { out.Printf("test   ") },
		},
		{
			name: "Println",
			fn:   func() { out.Println("test   ") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			tt.fn()

			got := stdout.String()
			// Should not have trailing spaces before the newline (if present)
			if strings.HasSuffix(strings.TrimSuffix(got, "\n"), " ") {
				t.Errorf("%s with trimming has trailing spaces: %q", tt.name, got)
			}
		})
	}
}

// Helper to create IO context with buffers for testing output.
func createTestIOContextWithBuffers() (iolib.Context, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	streams := &mockStreams{
		output: stdout,
		error:  stderr,
	}

	ctx, _ := iolib.NewContext(
		iolib.WithStreams(streams),
	)

	return ctx, stdout, stderr
}

// mockStreams implements iolib.Streams for testing.
type mockStreams struct {
	output *bytes.Buffer
	error  *bytes.Buffer
}

func (m *mockStreams) Input() stdIo.Reader {
	return nil
}

func (m *mockStreams) Output() stdIo.Writer {
	return m.output
}

func (m *mockStreams) Error() stdIo.Writer {
	return m.error
}

func (m *mockStreams) RawOutput() stdIo.Writer {
	return m.output
}

func (m *mockStreams) RawError() stdIo.Writer {
	return m.error
}
