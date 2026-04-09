package data

import (
	"bytes"
	"encoding/json"
	stdio "io"
	"testing"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestWrite(t *testing.T) {
	// Setup test I/O context with buffers.
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

	// Initialize global writer.
	InitWriter(ioCtx)

	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "simple text",
			content: "hello world",
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "empty string",
			content: "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "multiline text",
			content: "line1\nline2\nline3",
			want:    "line1\nline2\nline3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Write(tt.content)

			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Write() output = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("Write() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

func TestWritef(t *testing.T) {
	// Setup test I/O context with buffers.
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

	// Initialize global writer.
	InitWriter(ioCtx)

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
			format:  "name=%s, count=%d, price=%.2f",
			args:    []interface{}{"widget", 42, 99.99},
			want:    "name=widget, count=42, price=99.99",
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

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Writef() output = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("Writef() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

func TestWriteln(t *testing.T) {
	// Setup test I/O context with buffers.
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

	// Initialize global writer.
	InitWriter(ioCtx)

	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "simple text with newline",
			content: "hello world",
			want:    "hello world\n",
			wantErr: false,
		},
		{
			name:    "empty string with newline",
			content: "",
			want:    "\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Writeln(tt.content)

			if (err != nil) != tt.wantErr {
				t.Errorf("Writeln() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Writeln() output = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("Writeln() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

// setupTestIO creates test I/O context with buffers.
func setupTestIO(t *testing.T) (stdout, stderr *bytes.Buffer, cleanup func()) {
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

	// Save old context.
	ioMu.Lock()
	oldCtx := globalIOContext
	ioMu.Unlock()

	// Initialize global writer.
	InitWriter(ioCtx)

	// Return cleanup function to restore old context.
	cleanup = func() {
		ioMu.Lock()
		globalIOContext = oldCtx
		ioMu.Unlock()
	}

	return stdout, stderr, cleanup
}

//nolint:dupl // Test structure is similar to TestWriteYAML but tests different marshaling.
func TestWriteJSON(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	type testStruct struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "struct with fields",
			input:   testStruct{Name: "test", Count: 42},
			wantErr: false,
		},
		{
			name:    "map",
			input:   map[string]interface{}{"key": "value", "num": 123},
			wantErr: false,
		},
		{
			name:    "array",
			input:   []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "nil",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := WriteJSON(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("WriteJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify valid JSON was written.
			if !tt.wantErr {
				var result interface{}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Errorf("WriteJSON() output is not valid JSON: %v, output: %s", err, stdout.String())
				}
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("WriteJSON() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

//nolint:dupl // Test structure is similar to TestWriteJSON but tests different marshaling.
func TestWriteYAML(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	type testStruct struct {
		Name  string `yaml:"name"`
		Count int    `yaml:"count"`
	}

	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "struct with fields",
			input:   testStruct{Name: "test", Count: 42},
			wantErr: false,
		},
		{
			name:    "map",
			input:   map[string]interface{}{"key": "value", "num": 123},
			wantErr: false,
		},
		{
			name:    "array",
			input:   []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "nil",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := WriteYAML(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("WriteYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify valid YAML was written.
			if !tt.wantErr {
				var result interface{}
				if err := yaml.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Errorf("WriteYAML() output is not valid YAML: %v, output: %s", err, stdout.String())
				}
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("WriteYAML() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

func TestGetIOContext_Panic(t *testing.T) {
	// Save current global context.
	ioMu.Lock()
	oldCtx := globalIOContext
	globalIOContext = nil
	ioMu.Unlock()

	// Restore after test.
	defer func() {
		ioMu.Lock()
		globalIOContext = oldCtx
		ioMu.Unlock()
	}()

	// Should panic when not initialized.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("getIOContext() did not panic when globalIOContext is nil")
		}
	}()

	getIOContext()
}

// mockMarkdownRenderer is a test implementation of MarkdownRenderer.
type mockMarkdownRenderer struct {
	renderFunc func(string) (string, error)
}

func (m *mockMarkdownRenderer) Markdown(content string) (string, error) {
	if m.renderFunc != nil {
		return m.renderFunc(content)
	}
	return "rendered: " + content, nil
}

func TestMarkdown(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	// Setup mock renderer.
	mockRenderer := &mockMarkdownRenderer{}
	SetMarkdownRenderer(mockRenderer)

	tests := []struct {
		name       string
		content    string
		renderFunc func(string) (string, error)
		want       string
		wantErr    bool
	}{
		{
			name:    "simple markdown",
			content: "# Hello",
			want:    "rendered: # Hello",
			wantErr: false,
		},
		{
			name:    "empty content",
			content: "",
			want:    "rendered: ",
			wantErr: false,
		},
		{
			name:    "multiline markdown",
			content: "# Title\n\nParagraph with **bold**.",
			want:    "rendered: # Title\n\nParagraph with **bold**.",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			if tt.renderFunc != nil {
				mockRenderer.renderFunc = tt.renderFunc
			} else {
				mockRenderer.renderFunc = nil
			}

			err := Markdown(tt.content)

			if (err != nil) != tt.wantErr {
				t.Errorf("Markdown() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Markdown() output = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("Markdown() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

func TestMarkdownf(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	// Setup mock renderer.
	mockRenderer := &mockMarkdownRenderer{}
	SetMarkdownRenderer(mockRenderer)

	tests := []struct {
		name    string
		format  string
		args    []interface{}
		want    string
		wantErr bool
	}{
		{
			name:    "formatted markdown",
			format:  "# %s",
			args:    []interface{}{"Hello"},
			want:    "rendered: # Hello",
			wantErr: false,
		},
		{
			name:    "multiple arguments",
			format:  "## %s - Count: %d",
			args:    []interface{}{"Title", 42},
			want:    "rendered: ## Title - Count: 42",
			wantErr: false,
		},
		{
			name:    "no arguments",
			format:  "# Static Title",
			args:    []interface{}{},
			want:    "rendered: # Static Title",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()

			err := Markdownf(tt.format, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("Markdownf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := stdout.String()
			if got != tt.want {
				t.Errorf("Markdownf() output = %q, want %q", got, tt.want)
			}

			// Verify nothing written to stderr.
			if stderr.Len() != 0 {
				t.Errorf("Markdownf() wrote to stderr: %q", stderr.String())
			}
		})
	}
}

func TestMarkdown_RendererNotInitialized(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	// Save old renderer.
	ioMu.Lock()
	oldRenderer := globalMarkdownRender
	globalMarkdownRender = nil
	ioMu.Unlock()

	// Restore after test.
	defer func() {
		ioMu.Lock()
		globalMarkdownRender = oldRenderer
		ioMu.Unlock()
	}()

	err := Markdown("# Test")
	if err == nil {
		t.Error("Markdown() should return error when renderer not initialized")
	}

	// Should not have written anything.
	if stdout.Len() != 0 {
		t.Errorf("Markdown() wrote to stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("Markdown() wrote to stderr: %q", stderr.String())
	}
}

func TestMarkdown_RendererError(t *testing.T) {
	stdout, stderr, cleanup := setupTestIO(t)
	defer cleanup()

	// Setup mock renderer that returns error.
	mockRenderer := &mockMarkdownRenderer{
		renderFunc: func(content string) (string, error) {
			return "", errUtils.ErrUIFormatterNotInitialized
		},
	}
	SetMarkdownRenderer(mockRenderer)

	content := "# Test"
	err := Markdown(content)
	// Should write plain content when rendering fails.
	if err != nil {
		t.Errorf("Markdown() should degrade gracefully, got error: %v", err)
	}

	got := stdout.String()
	if got != content {
		t.Errorf("Markdown() output = %q, want %q (plain content)", got, content)
	}

	// Verify nothing written to stderr.
	if stderr.Len() != 0 {
		t.Errorf("Markdown() wrote to stderr: %q", stderr.String())
	}
}

func TestMarkdown_ContextNotInitialized(t *testing.T) {
	// Save current context.
	ioMu.Lock()
	oldCtx := globalIOContext
	globalIOContext = nil
	ioMu.Unlock()

	// Restore after test.
	defer func() {
		ioMu.Lock()
		globalIOContext = oldCtx
		ioMu.Unlock()
	}()

	// Should panic when context not initialized.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Markdown() did not panic when globalIOContext is nil")
		}
	}()

	_ = Markdown("# Test")
}
