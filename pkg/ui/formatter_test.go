package ui

import (
	"strings"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
)

func TestNewFormatter(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()

	f := NewFormatter(ioCtx, term)

	if f == nil {
		t.Fatal("NewFormatter() returned nil")
	}

	if f.Styles() == nil {
		t.Error("Styles() returned nil")
	}
}

func TestFormatter_SupportsColor(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		want    bool
	}{
		{
			name:    "ColorNone returns false",
			profile: terminal.ColorNone,
			want:    false,
		},
		{
			name:    "Color16 returns true",
			profile: terminal.Color16,
			want:    true,
		},
		{
			name:    "Color256 returns true",
			profile: terminal.Color256,
			want:    true,
		},
		{
			name:    "ColorTrue returns true",
			profile: terminal.ColorTrue,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.SupportsColor()
			if got != tt.want {
				t.Errorf("SupportsColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatter_ColorProfile(t *testing.T) {
	profiles := []terminal.ColorProfile{
		terminal.ColorNone,
		terminal.Color16,
		terminal.Color256,
		terminal.ColorTrue,
	}

	for _, profile := range profiles {
		t.Run("profile_"+string(rune(profile)), func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(profile)
			f := NewFormatter(ioCtx, term)

			got := f.ColorProfile()
			if got != profile {
				t.Errorf("ColorProfile() = %v, want %v", got, profile)
			}
		})
	}
}

func TestFormatter_Success(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: terminal.ColorNone,
			input:   "test",
		},
		{
			name:    "with color",
			profile: terminal.Color16,
			input:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.Success(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Success() = %q, doesn't contain input %q", got, tt.input)
			}

			// Output should always include checkmark icon (with or without color)
			expectedNoColor := "✓ " + tt.input
			if tt.profile == terminal.ColorNone && got != expectedNoColor {
				t.Errorf("Success() with no color = %q, want %q", got, expectedNoColor)
			}

			// Output should contain checkmark icon
			if !strings.Contains(got, "✓") {
				t.Errorf("Success() = %q, should contain checkmark icon", got)
			}
		})
	}
}

func TestFormatter_Warning(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	input := "warning message"
	got := f.Warning(input)

	// With color support, should contain the input
	if !strings.Contains(got, input) && f.SupportsColor() {
		t.Errorf("Warning() output doesn't contain input: %q", got)
	}
}

func TestFormatter_Error(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	input := "error message"
	got := f.Error(input)

	// With color support, should contain the input
	if !strings.Contains(got, input) && f.SupportsColor() {
		t.Errorf("Error() output doesn't contain input: %q", got)
	}
}

func TestFormatter_Info(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	input := "info message"
	got := f.Info(input)

	// With color support, should contain the input
	if !strings.Contains(got, input) && f.SupportsColor() {
		t.Errorf("Info() output doesn't contain input: %q", got)
	}
}

func TestFormatter_Muted(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: terminal.ColorNone,
			input:   "muted text",
		},
		{
			name:    "with color",
			profile: terminal.Color16,
			input:   "muted text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.Muted(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Muted() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == terminal.ColorNone && got != tt.input {
				t.Errorf("Muted() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Bold(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: terminal.ColorNone,
			input:   "bold text",
		},
		{
			name:    "with color",
			profile: terminal.Color16,
			input:   "bold text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.Bold(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Bold() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == terminal.ColorNone && got != tt.input {
				t.Errorf("Bold() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Heading(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: terminal.ColorNone,
			input:   "heading",
		},
		{
			name:    "with color",
			profile: terminal.Color16,
			input:   "heading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.Heading(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Heading() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == terminal.ColorNone && got != tt.input {
				t.Errorf("Heading() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Label(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: terminal.ColorNone,
			input:   "label",
		},
		{
			name:    "with color",
			profile: terminal.Color16,
			input:   "label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			got := f.Label(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Label() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == terminal.ColorNone && got != tt.input {
				t.Errorf("Label() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_RenderMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple markdown",
			input:   "# Heading\n\nParagraph",
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: false,
		},
		{
			name:    "with code block",
			input:   "```go\nfunc main() {}\n```",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := terminal.New()
			f := NewFormatter(ioCtx, term)

			got, err := f.Markdown(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Markdown() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got == "" && tt.input != "" {
				t.Error("Markdown() returned empty string for non-empty input")
			}
		})
	}
}

func TestFormatter_Markdown_MaxWidth(t *testing.T) {
	// Test that Markdown doesn't fail with markdown content
	// This test ensures the method handles content correctly
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	input := "# Test\n\nThis is a very long line that should be wrapped according to the terminal width."
	got, err := f.Markdown(input)
	if err != nil {
		t.Errorf("Markdown() error = %v", err)
	}

	if got == "" {
		t.Error("Markdown() returned empty string")
	}
}

// Helper functions for testing.

func createTestIOContext() iolib.Context {
	ctx, _ := iolib.NewContext()
	return ctx
}

func createMockTerminal(profile terminal.ColorProfile) terminal.Terminal {
	return &mockTerminal{profile: profile}
}

// mockTerminal implements terminal.Terminal for testing.
type mockTerminal struct {
	profile terminal.ColorProfile
	width   int
	height  int
	isTTY   bool
}

func (m *mockTerminal) Write(content string) error {
	// No-op for tests - just discard output
	return nil
}

func (m *mockTerminal) IsTTY(stream terminal.Stream) bool {
	return m.isTTY
}

func (m *mockTerminal) ColorProfile() terminal.ColorProfile {
	return m.profile
}

func (m *mockTerminal) Width(stream terminal.Stream) int {
	return m.width
}

func (m *mockTerminal) Height(stream terminal.Stream) int {
	return m.height
}

func (m *mockTerminal) SetTitle(title string) {}

func (m *mockTerminal) RestoreTitle() {}

func (m *mockTerminal) Alert() {}

func TestFormatter_StatusMessage(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		icon    string
		text    string
		want    string
	}{
		{
			name:    "no color - plain formatting",
			profile: terminal.ColorNone,
			icon:    "✓",
			text:    "test message",
			want:    "✓ test message",
		},
		{
			name:    "with color - contains icon and text",
			profile: terminal.Color16,
			icon:    "✗",
			text:    "error message",
			// With color, output will have ANSI codes, so just check it contains the parts
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term)

			successStyle := f.Styles().Success
			got := f.StatusMessage(tt.icon, &successStyle, tt.text)

			// For no color, output should match exactly
			if tt.profile == terminal.ColorNone && got != tt.want {
				t.Errorf("StatusMessage() = %q, want %q", got, tt.want)
			}

			// For any profile, output should contain icon and text
			if !strings.Contains(got, tt.icon) {
				t.Errorf("StatusMessage() = %q, should contain icon %q", got, tt.icon)
			}
			if !strings.Contains(got, tt.text) {
				t.Errorf("StatusMessage() = %q, should contain text %q", got, tt.text)
			}
		})
	}
}

func TestFormatter_AutomaticIcons(t *testing.T) {
	tests := []struct {
		name         string
		method       func(f Formatter, text string) string
		expectedIcon string
		text         string
	}{
		{
			name:         "Success includes checkmark",
			method:       func(f Formatter, text string) string { return f.Success(text) },
			expectedIcon: "✓",
			text:         "operation complete",
		},
		{
			name:         "Error includes X mark",
			method:       func(f Formatter, text string) string { return f.Error(text) },
			expectedIcon: "✗",
			text:         "operation failed",
		},
		{
			name:         "Warning includes warning sign",
			method:       func(f Formatter, text string) string { return f.Warning(text) },
			expectedIcon: "⚠",
			text:         "potential issue",
		},
		{
			name:         "Info includes info icon",
			method:       func(f Formatter, text string) string { return f.Info(text) },
			expectedIcon: "ℹ",
			text:         "information message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with color
			ioCtx := createTestIOContext()
			term := createMockTerminal(terminal.Color16)
			f := NewFormatter(ioCtx, term)

			got := tt.method(f, tt.text)

			if !strings.Contains(got, tt.expectedIcon) {
				t.Errorf("%s output = %q, should contain icon %q", tt.name, got, tt.expectedIcon)
			}
			if !strings.Contains(got, tt.text) {
				t.Errorf("%s output = %q, should contain text %q", tt.name, got, tt.text)
			}

			// Test without color
			ioCtxNoColor := createTestIOContext()
			termNoColor := createMockTerminal(terminal.ColorNone)
			fNoColor := NewFormatter(ioCtxNoColor, termNoColor)

			gotNoColor := tt.method(fNoColor, tt.text)
			expectedNoColor := tt.expectedIcon + " " + tt.text

			if gotNoColor != expectedNoColor {
				t.Errorf("%s with no color = %q, want %q", tt.name, gotNoColor, expectedNoColor)
			}
		})
	}
}

func TestFormatter_FormattedMethods(t *testing.T) {
	tests := []struct {
		name         string
		method       func(f Formatter) string
		expectedIcon string
		expectedText string
	}{
		{
			name:         "Successf formats with arguments",
			method:       func(f Formatter) string { return f.Successf("Processed %d items in %s", 42, "5s") },
			expectedIcon: "✓",
			expectedText: "Processed 42 items in 5s",
		},
		{
			name:         "Errorf formats with arguments",
			method:       func(f Formatter) string { return f.Errorf("Failed to connect to %s on port %d", "localhost", 8080) },
			expectedIcon: "✗",
			expectedText: "Failed to connect to localhost on port 8080",
		},
		{
			name:         "Warningf formats with arguments",
			method:       func(f Formatter) string { return f.Warningf("Found %d deprecated configs", 3) },
			expectedIcon: "⚠",
			expectedText: "Found 3 deprecated configs",
		},
		{
			name:         "Infof formats with arguments",
			method:       func(f Formatter) string { return f.Infof("Loading configuration from %s", "/etc/atmos.yaml") },
			expectedIcon: "ℹ",
			expectedText: "Loading configuration from /etc/atmos.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with color
			ioCtx := createTestIOContext()
			term := createMockTerminal(terminal.Color16)
			f := NewFormatter(ioCtx, term)

			got := tt.method(f)

			if !strings.Contains(got, tt.expectedIcon) {
				t.Errorf("%s output = %q, should contain icon %q", tt.name, got, tt.expectedIcon)
			}
			if !strings.Contains(got, tt.expectedText) {
				t.Errorf("%s output = %q, should contain text %q", tt.name, got, tt.expectedText)
			}

			// Test without color
			ioCtxNoColor := createTestIOContext()
			termNoColor := createMockTerminal(terminal.ColorNone)
			fNoColor := NewFormatter(ioCtxNoColor, termNoColor)

			gotNoColor := tt.method(fNoColor)
			expectedNoColor := tt.expectedIcon + " " + tt.expectedText

			if gotNoColor != expectedNoColor {
				t.Errorf("%s with no color = %q, want %q", tt.name, gotNoColor, expectedNoColor)
			}
		})
	}
}

func TestFormatter_FormatToast_SingleLine(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name     string
		icon     string
		message  string
		expected string
	}{
		{
			name:     "simple single line",
			icon:     "✓",
			message:  "Done",
			expected: "✓ Done\n",
		},
		{
			name:     "emoji icon",
			icon:     "📦",
			message:  "Package installed",
			expected: "📦 Package installed\n",
		},
		{
			name:     "multi-character emoji",
			icon:     "🔧",
			message:  "Tool configured",
			expected: "🔧 Tool configured\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.formatToast(tt.icon, tt.message)
			if got != tt.expected {
				t.Errorf("formatToast() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatter_FormatToast_Multiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name     string
		icon     string
		message  string
		expected string
	}{
		{
			name:     "two lines",
			icon:     "✓",
			message:  "Installation complete\nVersion: 1.2.3",
			expected: "✓ Installation complete\n  Version: 1.2.3\n",
		},
		{
			name:     "three lines",
			icon:     "✓",
			message:  "Done\nFile: test.txt\nSize: 1.2MB",
			expected: "✓ Done\n  File: test.txt\n  Size: 1.2MB\n",
		},
		{
			name:     "emoji icon multiline",
			icon:     "📦",
			message:  "Package installed\nName: atmos\nVersion: 1.2.3",
			expected: "📦 Package installed\n  Name: atmos\n  Version: 1.2.3\n",
		},
		{
			name:     "error icon multiline",
			icon:     "✗",
			message:  "Installation failed\nReason: Network timeout\nRetry suggested",
			expected: "✗ Installation failed\n  Reason: Network timeout\n  Retry suggested\n",
		},
		{
			name:     "empty lines preserved",
			icon:     "ℹ",
			message:  "Processing\n\nComplete",
			expected: "ℹ Processing\n  \n  Complete\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.formatToast(tt.icon, tt.message)
			if got != tt.expected {
				t.Errorf("formatToast() = %q, want %q", got, tt.expected)
				// Show visual diff
				t.Logf("Got:\n%s", got)
				t.Logf("Want:\n%s", tt.expected)
			}
		})
	}
}

func TestFormatter_FormatToast_UnicodeWidth(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name        string
		icon        string
		message     string
		description string
	}{
		{
			name:        "single char icon",
			icon:        "✓",
			message:     "Line 1\nLine 2",
			description: "Simple checkmark has width 1",
		},
		{
			name:        "emoji icon",
			icon:        "📦",
			message:     "Line 1\nLine 2",
			description: "Emoji typically has width 2",
		},
		{
			name:        "double width icon",
			icon:        "🔧",
			message:     "Line 1\nLine 2",
			description: "Wrench emoji has width 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.formatToast(tt.icon, tt.message)

			// Verify that continuation lines are indented
			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			if len(lines) < 2 {
				t.Fatalf("Expected at least 2 lines, got %d", len(lines))
			}

			// First line should start with icon
			if !strings.HasPrefix(lines[0], tt.icon) {
				t.Errorf("First line should start with icon %q, got %q", tt.icon, lines[0])
			}

			// Second line should be indented (start with space)
			if !strings.HasPrefix(lines[1], " ") {
				t.Errorf("Second line should be indented, got %q", lines[1])
			}

			// Calculate expected indent based on icon rune count
			iconWidth := len([]rune(tt.icon))
			expectedIndent := strings.Repeat(" ", iconWidth+1)

			// Verify indent matches
			if !strings.HasPrefix(lines[1], expectedIndent) {
				t.Errorf("Second line indent = %d spaces, want %d spaces (icon width %d)",
					len(lines[1])-len(strings.TrimLeft(lines[1], " ")),
					iconWidth+1,
					iconWidth)
			}
		})
	}
}

func TestToast_Integration(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	InitFormatter(ioCtx)
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name     string
		icon     string
		message  string
		wantErr  bool
		contains []string
	}{
		{
			name:     "single line toast",
			icon:     "✓",
			message:  "Success",
			wantErr:  false,
			contains: []string{"✓", "Success"},
		},
		{
			name:     "multiline toast",
			icon:     "📦",
			message:  "Installed\nVersion: 1.0",
			wantErr:  false,
			contains: []string{"📦", "Installed", "Version: 1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Toast(tt.icon, tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("Toast() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToastf_Integration(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	InitFormatter(ioCtx)
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name    string
		icon    string
		format  string
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "formatted single line",
			icon:    "✓",
			format:  "Installed %s version %s",
			args:    []interface{}{"atmos", "1.2.3"},
			wantErr: false,
		},
		{
			name:    "formatted multiline",
			icon:    "📦",
			format:  "Package: %s\nVersion: %s\nSize: %dMB",
			args:    []interface{}{"atmos", "1.2.3", 42},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Toastf(tt.icon, tt.format, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Toastf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatter_FormatToast_EdgeCases(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name     string
		icon     string
		message  string
		expected string
	}{
		{
			name:     "empty message",
			icon:     "✓",
			message:  "",
			expected: "✓ \n",
		},
		{
			name:     "message with only newline",
			icon:     "✓",
			message:  "\n",
			expected: "✓ \n  \n",
		},
		{
			name:     "message starting with newline",
			icon:     "✓",
			message:  "\nStarting text",
			expected: "✓ \n  Starting text\n",
		},
		{
			name:     "message ending with newline",
			icon:     "✓",
			message:  "Ending text\n",
			expected: "✓ Ending text\n  \n",
		},
		{
			name:     "multiple consecutive newlines",
			icon:     "ℹ",
			message:  "Line 1\n\n\nLine 2",
			expected: "ℹ Line 1\n  \n  \n  Line 2\n",
		},
		{
			name:     "long multiline message",
			icon:     "📋",
			message:  "Task 1\nTask 2\nTask 3\nTask 4\nTask 5",
			expected: "📋 Task 1\n  Task 2\n  Task 3\n  Task 4\n  Task 5\n",
		},
		{
			name:     "special characters in message",
			icon:     "⚠",
			message:  "Warning: special chars\n\t- Tab character\n  - Spaces",
			expected: "⚠ Warning: special chars\n  \t- Tab character\n    - Spaces\n",
		},
		{
			name:     "unicode in message",
			icon:     "✓",
			message:  "Unicode: 你好\n世界",
			expected: "✓ Unicode: 你好\n  世界\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.formatToast(tt.icon, tt.message)
			if got != tt.expected {
				t.Errorf("formatToast() = %q, want %q", got, tt.expected)
				// Show visual diff
				t.Logf("Got:\n%s", got)
				t.Logf("Want:\n%s", tt.expected)
			}
		})
	}
}

func TestFormatter_FormatToast_RealWorldExamples(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name        string
		icon        string
		message     string
		description string
	}{
		{
			name:        "installation success",
			icon:        "✓",
			message:     "Installation complete\nPackage: terraform\nVersion: 1.5.0\nLocation: /usr/local/bin/terraform",
			description: "Multi-line installation summary",
		},
		{
			name:        "error with details",
			icon:        "✗",
			message:     "Deployment failed\nReason: Connection timeout\nHost: example.com\nRetry: Run 'atmos deploy' again",
			description: "Multi-line error message",
		},
		{
			name:        "progress update",
			icon:        "📦",
			message:     "Processing components\nFound: 15 stacks\nProcessing: ue2-prod\nStatus: validating",
			description: "Multi-line progress notification",
		},
		{
			name:        "configuration info",
			icon:        "ℹ",
			message:     "Configuration loaded\nFile: atmos.yaml\nStacks: 42\nComponents: 156",
			description: "Multi-line configuration summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.formatToast(tt.icon, tt.message)

			// Verify structure
			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			if len(lines) < 2 {
				t.Errorf("Expected multiline output for %s, got single line", tt.description)
			}

			// First line should have icon
			if !strings.HasPrefix(lines[0], tt.icon) {
				t.Errorf("First line should start with icon %q, got %q", tt.icon, lines[0])
			}

			// All continuation lines should be indented
			for i := 1; i < len(lines); i++ {
				if !strings.HasPrefix(lines[i], " ") {
					t.Errorf("Line %d should be indented, got %q", i+1, lines[i])
				}
			}

			// Verify ends with newline
			if !strings.HasSuffix(got, "\n") {
				t.Error("Output should end with newline")
			}
		})
	}
}

func TestToast_WithConvenienceFunctions(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	InitFormatter(ioCtx)
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name     string
		fn       func() error
		contains []string
	}{
		{
			name: "Success with multiline via Success",
			fn: func() error {
				return Success("Done\nAll tasks completed")
			},
			contains: []string{"✓", "Done", "All tasks completed"},
		},
		{
			name: "Error with multiline via Error",
			fn: func() error {
				return Error("Failed\nCheck logs for details")
			},
			contains: []string{"✗", "Failed", "Check logs"},
		},
		{
			name: "Warning with multiline via Warning",
			fn: func() error {
				return Warning("Deprecated\nUse new API instead")
			},
			contains: []string{"⚠", "Deprecated", "new API"},
		},
		{
			name: "Info with multiline via Info",
			fn: func() error {
				return Info("Processing\nStep 1 of 3")
			},
			contains: []string{"ℹ", "Processing", "Step 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != nil {
				t.Errorf("Function returned error: %v", err)
			}
		})
	}
}

func TestToastf_FormattingWithMultiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	InitFormatter(ioCtx)
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)

	tests := []struct {
		name   string
		icon   string
		format string
		args   []interface{}
	}{
		{
			name:   "formatted with embedded newlines",
			icon:   "✓",
			format: "Installed: %s\nVersion: %s\nSize: %d MB",
			args:   []interface{}{"atmos", "1.2.3", 42},
		},
		{
			name:   "formatted with multiple types",
			icon:   "📊",
			format: "Stats\nProcessed: %d files\nDuration: %.2f seconds\nSuccess rate: %d%%",
			args:   []interface{}{150, 12.34, 98},
		},
		{
			name:   "formatted with boolean",
			icon:   "🔍",
			format: "Validation\nPassed: %t\nErrors: %d",
			args:   []interface{}{true, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Toastf(tt.icon, tt.format, tt.args...)
			if err != nil {
				t.Errorf("Toastf() returned error: %v", err)
			}
		})
	}
}

func TestFormatter_FormatToast_NotInitialized(t *testing.T) {
	// Temporarily clear global formatter
	formatterMu.Lock()
	oldFormatter := globalFormatter
	globalFormatter = nil
	formatterMu.Unlock()

	// Restore after test
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		formatterMu.Unlock()
	}()

	err := Toast("✓", "This should fail")
	if err == nil {
		t.Error("Expected error when formatter not initialized, got nil")
	}

	err = Toastf("✓", "This should fail: %s", "test")
	if err == nil {
		t.Error("Expected error when formatter not initialized for Toastf, got nil")
	}
}
