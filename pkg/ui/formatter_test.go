package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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
			// Strip ANSI codes before checking text containment (Glamour wraps each char in styling)
			plainText := ansi.Strip(got)
			if !strings.Contains(plainText, tt.text) {
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
			// Strip ANSI codes before checking text containment (Glamour wraps each char in styling)
			plainText := ansi.Strip(got)
			if !strings.Contains(plainText, tt.expectedText) {
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
			got := f.Toast(tt.icon, tt.message)
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
			expected: "📦 Package installed\n   Name: atmos\n   Version: 1.2.3\n", // 3 spaces: emoji is 2 cells + 1
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
			got := f.Toast(tt.icon, tt.message)
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
			got := f.Toast(tt.icon, tt.message)

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

			// Calculate expected indent using lipgloss.Width() - same as production code
			iconWidth := lipgloss.Width(tt.icon)
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
		contains []string
	}{
		{
			name:     "single line toast",
			icon:     "✓",
			message:  "Success",
			contains: []string{"✓", "Success"},
		},
		{
			name:     "multiline toast",
			icon:     "📦",
			message:  "Installed\nVersion: 1.0",
			contains: []string{"📦", "Installed", "Version: 1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Toast no longer returns an error - it logs internally if there's a write failure.
			Toast(tt.icon, tt.message)
		})
	}
}

func TestToastf_Integration(t *testing.T) {
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
			name:   "formatted single line",
			icon:   "✓",
			format: "Installed %s version %s",
			args:   []interface{}{"atmos", "1.2.3"},
		},
		{
			name:   "formatted multiline",
			icon:   "📦",
			format: "Package: %s\nVersion: %s\nSize: %dMB",
			args:   []interface{}{"atmos", "1.2.3", 42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Toastf no longer returns an error - it logs internally if there's a write failure.
			Toastf(tt.icon, tt.format, tt.args...)
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
			expected: "✓ \n  \n", // Glamour renders empty as paragraph with indent
		},
		{
			name:     "message with only newline",
			icon:     "✓",
			message:  "\n",
			expected: "✓ \n  \n", // Glamour collapses single newline to empty paragraph
		},
		{
			name:     "message starting with newline",
			icon:     "✓",
			message:  "\nStarting text",
			expected: "✓ Starting text\n", // Glamour doesn't preserve leading newline in markdown
		},
		{
			name:     "message ending with newline",
			icon:     "✓",
			message:  "Ending text\n",
			expected: "✓ Ending text\n  \n", // Glamour treats trailing newline as paragraph separation
		},
		{
			name:     "multiple consecutive newlines",
			icon:     "ℹ",
			message:  "Line 1\n\n\nLine 2",
			expected: "ℹ Line 1\n  \n  Line 2\n", // Glamour collapses multiple newlines to single empty line
		},
		{
			name:     "long multiline message",
			icon:     "📋",
			message:  "Task 1\nTask 2\nTask 3\nTask 4\nTask 5",
			expected: "📋 Task 1\n   Task 2\n   Task 3\n   Task 4\n   Task 5\n", // 3 spaces: emoji is 2 cells + 1
		},
		{
			name:     "special characters in message",
			icon:     "⚠",
			message:  "Warning: special chars\n\t- Tab character\n  - Spaces",
			expected: "⚠ Warning: special chars\n  - Tab character\n  \n  • Spaces\n", // Glamour normalizes whitespace, adds paragraph break, and converts "  - " to bullet
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
			got := f.Toast(tt.icon, tt.message)
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
			got := f.Toast(tt.icon, tt.message)

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
		fn       func()
		contains []string
	}{
		{
			name: "Success with multiline via Success",
			fn: func() {
				Success("Done\nAll tasks completed")
			},
			contains: []string{"✓", "Done", "All tasks completed"},
		},
		{
			name: "Error with multiline via Error",
			fn: func() {
				Error("Failed\nCheck logs for details")
			},
			contains: []string{"✗", "Failed", "Check logs"},
		},
		{
			name: "Warning with multiline via Warning",
			fn: func() {
				Warning("Deprecated\nUse new API instead")
			},
			contains: []string{"⚠", "Deprecated", "new API"},
		},
		{
			name: "Info with multiline via Info",
			fn: func() {
				Info("Processing\nStep 1 of 3")
			},
			contains: []string{"ℹ", "Processing", "Step 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Functions no longer return errors - they log internally if there's a write failure.
			tt.fn()
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
			// Toastf no longer returns an error - it logs internally if there's a write failure.
			Toastf(tt.icon, tt.format, tt.args...)
		})
	}
}

func TestFormatter_FormatToast_NotInitialized(t *testing.T) {
	// Temporarily clear global formatter and Format
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	globalFormatter = nil
	Format = nil
	formatterMu.Unlock()

	// Restore after test
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		formatterMu.Unlock()
	}()

	// Functions no longer return errors when not initialized - they log internally.
	// This test verifies that calling the functions when not initialized doesn't panic.
	Toast("✓", "This should not panic")
	Toastf("✓", "This should not panic: %s", "test")
}

func TestFormatter_ConvenienceFunctions_Multiline(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(*formatter, string) string
		message string
		icon    string
	}{
		{
			name:    "Success multiline",
			fn:      func(f *formatter, msg string) string { return f.Success(msg) },
			message: "Done\nAll complete",
			icon:    "✓",
		},
		{
			name:    "Error multiline",
			fn:      func(f *formatter, msg string) string { return f.Error(msg) },
			message: "Failed\nCheck logs",
			icon:    "✗",
		},
		{
			name:    "Warning multiline",
			fn:      func(f *formatter, msg string) string { return f.Warning(msg) },
			message: "Deprecated\nUse new API",
			icon:    "⚠",
		},
		{
			name:    "Info multiline",
			fn:      func(f *formatter, msg string) string { return f.Info(msg) },
			message: "Processing\nStep 1 of 3",
			icon:    "ℹ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with color
			ioCtx := createTestIOContext()
			term := createMockTerminal(terminal.ColorTrue)
			f := NewFormatter(ioCtx, term).(*formatter)

			result := tt.fn(f, tt.message)
			lines := strings.Split(result, "\n")

			if len(lines) < 2 {
				t.Errorf("Expected multiline output, got single line: %q", result)
			}

			// First line should contain the icon
			if !strings.Contains(lines[0], tt.icon) {
				t.Errorf("First line should contain icon %q, got %q", tt.icon, lines[0])
			}

			// Second line should be indented (strip ANSI codes first)
			plainLine := ansi.Strip(lines[1])
			if !strings.HasPrefix(plainLine, " ") {
				t.Errorf("Second line should be indented, got %q", lines[1])
			}

			// Test without color
			termNoColor := createMockTerminal(terminal.ColorNone)
			fNoColor := NewFormatter(ioCtx, termNoColor).(*formatter)

			resultNoColor := tt.fn(fNoColor, tt.message)
			linesNoColor := strings.Split(resultNoColor, "\n")

			if len(linesNoColor) < 2 {
				t.Errorf("Expected multiline output without color, got single line: %q", resultNoColor)
			}

			// Should still have icon and indentation
			if !strings.Contains(linesNoColor[0], tt.icon) {
				t.Errorf("First line without color should contain icon %q, got %q", tt.icon, linesNoColor[0])
			}

			if !strings.HasPrefix(linesNoColor[1], " ") {
				t.Errorf("Second line without color should be indented, got %q", linesNoColor[1])
			}
		})
	}
}

func TestFormatter_LipglossWidth(t *testing.T) {
	// Test that lipgloss.Width correctly handles ANSI codes and multi-cell characters.
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "plain text",
			input:    "hello",
			expected: 5,
		},
		{
			name:     "plain icon",
			input:    "✓",
			expected: 1,
		},
		{
			name:     "colored icon with ANSI",
			input:    "\x1b[32m✓\x1b[0m",
			expected: 1, // ANSI codes should not count
		},
		{
			name:     "emoji",
			input:    "📦",
			expected: 2, // Wide character
		},
		{
			name:     "colored emoji",
			input:    "\x1b[31m📦\x1b[0m",
			expected: 2, // ANSI codes stripped, emoji is 2 cells
		},
		{
			name:     "multiple colors",
			input:    "\x1b[31mred\x1b[0m and \x1b[32mgreen\x1b[0m",
			expected: 13, // "red and green"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lipgloss.Width(tt.input)
			if got != tt.expected {
				t.Errorf("lipgloss.Width(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatter_Successf_Multiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	result := f.Successf("Installed: %s\nVersion: %s", "atmos", "1.2.3")
	lines := strings.Split(result, "\n")

	if len(lines) < 2 {
		t.Errorf("Expected multiline output, got: %q", result)
	}

	if !strings.Contains(lines[0], "Installed: atmos") {
		t.Errorf("First line should contain formatted text, got: %q", lines[0])
	}

	if !strings.Contains(lines[1], "Version: 1.2.3") {
		t.Errorf("Second line should contain formatted text, got: %q", lines[1])
	}
}

func TestFormatter_Errorf_Multiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	result := f.Errorf("Failed: %s\nReason: %s", "deployment", "timeout")
	lines := strings.Split(result, "\n")

	if len(lines) < 2 {
		t.Errorf("Expected multiline output, got: %q", result)
	}

	if !strings.Contains(lines[0], "Failed: deployment") {
		t.Errorf("First line should contain formatted text, got: %q", lines[0])
	}

	if !strings.Contains(lines[1], "Reason: timeout") {
		t.Errorf("Second line should contain formatted text, got: %q", lines[1])
	}
}

func TestFormatter_Warningf_Multiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	result := f.Warningf("Deprecated: %s\nUse: %s instead", "old_api", "new_api")
	lines := strings.Split(result, "\n")

	if len(lines) < 2 {
		t.Errorf("Expected multiline output, got: %q", result)
	}

	if !strings.Contains(lines[0], "Deprecated: old_api") {
		t.Errorf("First line should contain formatted text, got: %q", lines[0])
	}

	if !strings.Contains(lines[1], "Use: new_api instead") {
		t.Errorf("Second line should contain formatted text, got: %q", lines[1])
	}
}

func TestFormatter_Infof_Multiline(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	result := f.Infof("Processing: %d items\nCompleted: %d%%", 100, 50)
	lines := strings.Split(result, "\n")

	if len(lines) < 2 {
		t.Errorf("Expected multiline output, got: %q", result)
	}

	if !strings.Contains(lines[0], "Processing: 100 items") {
		t.Errorf("First line should contain formatted text, got: %q", lines[0])
	}

	if !strings.Contains(lines[1], "Completed: 50%") {
		t.Errorf("Second line should contain formatted text, got: %q", lines[1])
	}
}

func TestTrimTrailingWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single line with trailing spaces",
			input:    "hello world     ",
			expected: []string{"hello world"},
		},
		{
			name:     "single line without trailing spaces",
			input:    "hello world",
			expected: []string{"hello world"},
		},
		{
			name:     "multiple lines with trailing spaces",
			input:    "line one     \nline two     \nline three     ",
			expected: []string{"line one", "line two", "line three"},
		},
		{
			name:     "empty line with only spaces (preserves 2-space indent)",
			input:    "line one\n     \nline three",
			expected: []string{"line one", "  ", "line three"},
		},
		{
			name:     "empty line with 2 spaces (preserves indent)",
			input:    "line one\n  \nline three",
			expected: []string{"line one", "  ", "line three"},
		},
		{
			name:     "empty line with 1 space (preserves single space)",
			input:    "line one\n \nline three",
			expected: []string{"line one", " ", "line three"},
		},
		{
			name:     "truly empty line (no spaces)",
			input:    "line one\n\nline three",
			expected: []string{"line one", "", "line three"},
		},
		{
			name:     "mixed: text with trailing spaces and empty lines",
			input:    "text     \n     \nmore text     ",
			expected: []string{"text", "  ", "more text"},
		},
		{
			name:     "leading spaces preserved, trailing spaces removed",
			input:    "  indented text     ",
			expected: []string{"  indented text"},
		},
		{
			name:     "multiple empty lines with spaces",
			input:    "start\n     \n     \nend",
			expected: []string{"start", "  ", "  ", "end"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "only spaces (becomes 2-space indent)",
			input:    "          ",
			expected: []string{"  "},
		},
		{
			name:     "tabs and spaces mixed (trims only trailing spaces, not tabs)",
			input:    "text\t\t   ",
			expected: []string{"text\t\t"},
		},
		{
			name:     "real Glamour output with 80-char padding",
			input:    "Success message" + strings.Repeat(" ", 65) + "\n  " + strings.Repeat(" ", 78),
			expected: []string{"Success message", "  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimTrailingWhitespace(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d lines, got %d lines", len(tt.expected), len(result))
				t.Errorf("Expected: %#v", tt.expected)
				t.Errorf("Got: %#v", result)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Line %d mismatch:\n  Expected: %q (len=%d)\n  Got:      %q (len=%d)",
						i, tt.expected[i], len(tt.expected[i]), result[i], len(result[i]))
				}
			}
		})
	}
}

func TestFormatSuccessAndError(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantIcon     string
		fallbackIcon string
		initFormat   bool
		formatFunc   func(string) string
		funcName     string
	}{
		{
			name:         "FormatSuccess with initialized formatter",
			text:         "Operation completed",
			wantIcon:     "✓",
			fallbackIcon: "✓",
			initFormat:   true,
			formatFunc:   FormatSuccess,
			funcName:     "FormatSuccess",
		},
		{
			name:         "FormatSuccess fallback when formatter not initialized",
			text:         "Operation completed",
			wantIcon:     "✓",
			fallbackIcon: "✓",
			initFormat:   false,
			formatFunc:   FormatSuccess,
			funcName:     "FormatSuccess",
		},
		{
			name:         "FormatError with initialized formatter",
			text:         "Operation failed",
			wantIcon:     "✗",
			fallbackIcon: "✗",
			initFormat:   true,
			formatFunc:   FormatError,
			funcName:     "FormatError",
		},
		{
			name:         "FormatError fallback when formatter not initialized",
			text:         "Operation failed",
			wantIcon:     "✗",
			fallbackIcon: "✗",
			initFormat:   false,
			formatFunc:   FormatError,
			funcName:     "FormatError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.initFormat {
				// Initialize formatter.
				ioCtx := createTestIOContext()
				term := createMockTerminal(terminal.ColorNone)
				InitFormatter(ioCtx)
				globalFormatter = NewFormatter(ioCtx, term).(*formatter)

				// Cleanup after test.
				defer func() {
					formatterMu.Lock()
					globalFormatter = nil
					Format = nil
					formatterMu.Unlock()
				}()
			} else {
				// Clear formatter.
				formatterMu.Lock()
				oldFormatter := globalFormatter
				oldFormat := Format
				globalFormatter = nil
				Format = nil
				formatterMu.Unlock()

				// Restore after test.
				defer func() {
					formatterMu.Lock()
					globalFormatter = oldFormatter
					Format = oldFormat
					formatterMu.Unlock()
				}()
			}

			got := tt.formatFunc(tt.text)

			// Should always contain the icon.
			if !strings.Contains(got, tt.wantIcon) {
				t.Errorf("%s() = %q, want icon %q", tt.funcName, got, tt.wantIcon)
			}

			// Should always contain the text.
			if !strings.Contains(got, tt.text) {
				t.Errorf("%s() = %q, want text %q", tt.funcName, got, tt.text)
			}

			if !tt.initFormat {
				// Fallback format should be "icon text".
				expected := tt.fallbackIcon + " " + tt.text
				if got != expected {
					t.Errorf("%s() fallback = %q, want %q", tt.funcName, got, expected)
				}
			}
		})
	}
}

func TestTrimRight(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{
			name:     "plain text no trailing spaces",
			input:    "hello world",
			expected: "hello world",
			desc:     "Baseline: plain text without trailing spaces should be unchanged",
		},
		{
			name:     "plain text with trailing spaces",
			input:    "hello world   ",
			expected: "hello world",
			desc:     "Plain text with trailing spaces should be trimmed",
		},
		{
			name:     "plain text with trailing tabs",
			input:    "hello world\t\t",
			expected: "hello world",
			desc:     "Plain text with trailing tabs should be trimmed",
		},
		{
			name:     "plain text with mixed trailing whitespace",
			input:    "hello world \t \t ",
			expected: "hello world",
			desc:     "Plain text with mixed trailing whitespace should be trimmed",
		},
		{
			name:     "ANSI colored text no trailing spaces",
			input:    "\x1b[38;2;247;250;252mhello world\x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI colored text without trailing spaces should preserve all codes",
		},
		{
			name:     "ANSI colored text with plain trailing spaces",
			input:    "\x1b[38;2;247;250;252mhello world\x1b[0m   ",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI colored text with plain trailing spaces should trim spaces",
		},
		{
			name:     "ANSI wrapped trailing spaces (Glamour pattern)",
			input:    "\x1b[38;2;247;250;252mhello world\x1b[0m\x1b[38;2;247;250;252m   \x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI-wrapped trailing spaces (Glamour padding) should be trimmed",
		},
		{
			name:     "ANSI wrapped trailing spaces complex",
			input:    "\x1b[38;2;247;250;252mhello\x1b[0m\x1b[38;2;100;100;100m world\x1b[0m\x1b[38;2;247;250;252m     \x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello\x1b[0m\x1b[38;2;100;100;100m world\x1b[0m",
			desc:     "Complex ANSI colored text with wrapped trailing spaces should trim only trailing portion",
		},
		{
			name:     "Unicode characters no trailing spaces",
			input:    "ℹ hello → world",
			expected: "ℹ hello → world",
			desc:     "Unicode characters should be handled correctly without trailing spaces",
		},
		{
			name:     "Unicode with ANSI and trailing spaces",
			input:    "\x1b[38;2;247;250;252mℹ hello → world\x1b[0m\x1b[38;2;247;250;252m   \x1b[0m",
			expected: "\x1b[38;2;247;250;252mℹ hello → world\x1b[0m",
			desc:     "Unicode with ANSI codes and wrapped trailing spaces should trim correctly",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			desc:     "Empty string should remain empty",
		},
		{
			name:     "only spaces",
			input:    "     ",
			expected: "",
			desc:     "String with only spaces should become empty",
		},
		{
			name:     "only ANSI wrapped spaces",
			input:    "\x1b[38;2;247;250;252m     \x1b[0m",
			expected: "",
			desc:     "String with only ANSI-wrapped spaces should become empty",
		},
		{
			name:     "preserves leading spaces",
			input:    "  hello world   ",
			expected: "  hello world",
			desc:     "Leading spaces should be preserved, only trailing removed",
		},
		{
			name:     "preserves ANSI on leading spaces",
			input:    "\x1b[38;2;247;250;252m  hello world\x1b[0m\x1b[38;2;247;250;252m   \x1b[0m",
			expected: "\x1b[38;2;247;250;252m  hello world\x1b[0m",
			desc:     "ANSI codes on leading spaces should be preserved",
		},
		{
			name:     "bold and colored text",
			input:    "\x1b[1m\x1b[38;2;247;250;252mBold text\x1b[0m\x1b[38;2;247;250;252m  \x1b[0m",
			expected: "\x1b[1m\x1b[38;2;247;250;252mBold text\x1b[0m",
			desc:     "Multiple ANSI codes (bold + color) should be preserved on content",
		},
		{
			name:     "real Glamour output pattern",
			input:    "\x1b[0m\x1b[38;2;247;250;252m\x1b[48;2;30;34;38m \x1b[0m\x1b[0m\x1b[1m\x1b[38;2;247;141;167mImage:\x1b[0m\x1b[0m\x1b[38;2;247;250;252m\x1b[48;2;30;34;38m cloudposse/geodesic:latest\x1b[0m\x1b[38;2;247;250;252m                                                \x1b[0m",
			expected: "\x1b[0m\x1b[38;2;247;250;252m\x1b[48;2;30;34;38m \x1b[0m\x1b[0m\x1b[1m\x1b[38;2;247;141;167mImage:\x1b[0m\x1b[0m\x1b[38;2;247;250;252m\x1b[48;2;30;34;38m cloudposse/geodesic:latest\x1b[0m",
			desc:     "Real Glamour output with 47+ trailing spaces should be trimmed correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimRight(tt.input)

			// Compare results
			if result != tt.expected {
				t.Errorf("\nTest: %s\nDescription: %s\n\nInput:\n  Raw: %q\n  Hex: % X\n  Visual: %s\n\nExpected:\n  Raw: %q\n  Hex: % X\n  Visual: %s\n\nGot:\n  Raw: %q\n  Hex: % X\n  Visual: %s",
					tt.name,
					tt.desc,
					tt.input,
					[]byte(tt.input),
					tt.input,
					tt.expected,
					[]byte(tt.expected),
					tt.expected,
					result,
					[]byte(result),
					result,
				)
			}

			// Additional verification: check visual width
			strippedInput := ansi.Strip(tt.input)
			strippedExpected := ansi.Strip(tt.expected)
			strippedResult := ansi.Strip(result)

			expectedWidth := ansi.StringWidth(strings.TrimRight(strippedInput, " \t"))
			resultWidth := ansi.StringWidth(strippedResult)

			if resultWidth != expectedWidth {
				t.Errorf("\nVisual width mismatch:\n  Expected trimmed width: %d (from %q)\n  Got width: %d (from %q)",
					expectedWidth,
					strings.TrimRight(strippedInput, " \t"),
					resultWidth,
					strippedResult,
				)
			}

			// Verify no trailing whitespace in result
			if strippedResult != strings.TrimRight(strippedResult, " \t") {
				t.Errorf("\nResult still has trailing whitespace:\n  Stripped result: %q\n  After TrimRight: %q",
					strippedResult,
					strings.TrimRight(strippedResult, " \t"),
				)
			}

			// Verify expected also matches this property
			if strippedExpected != strings.TrimRight(strippedExpected, " \t") {
				t.Errorf("\nTest case error - expected value has trailing whitespace:\n  Stripped expected: %q\n  After TrimRight: %q",
					strippedExpected,
					strings.TrimRight(strippedExpected, " \t"),
				)
			}
		})
	}
}

//nolint:dupl // Test structure intentionally mirrors TestTrimRight for consistency.
func TestTrimLeftSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{
			name:     "plain text no leading spaces",
			input:    "hello world",
			expected: "hello world",
			desc:     "Baseline: plain text without leading spaces should be unchanged",
		},
		{
			name:     "plain text with leading spaces",
			input:    "   hello world",
			expected: "hello world",
			desc:     "Plain text with leading spaces should be trimmed",
		},
		{
			name:     "ANSI colored text no leading spaces",
			input:    "\x1b[38;2;247;250;252mhello world\x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI colored text without leading spaces should preserve all codes",
		},
		{
			name:     "ANSI colored text with plain leading spaces",
			input:    "   \x1b[38;2;247;250;252mhello world\x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI colored text with plain leading spaces should trim spaces",
		},
		{
			name:     "ANSI codes before leading spaces (Glamour pattern)",
			input:    "\x1b[38;2;247;250;252m\x1b[0m\x1b[38;2;247;250;252m\x1b[0m  \x1b[38;2;247;250;252mhello world\x1b[0m",
			expected: "\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI codes before leading spaces (Glamour pattern) should be trimmed",
		},
		{
			name:     "ANSI wrapped leading spaces",
			input:    "\x1b[38;2;247;250;252m   \x1b[0m\x1b[38;2;247;250;252mhello world\x1b[0m",
			expected: "\x1b[0m\x1b[38;2;247;250;252mhello world\x1b[0m",
			desc:     "ANSI-wrapped leading spaces should be trimmed (reset code preserved)",
		},
		{
			name:     "mixed ANSI codes and spaces at start",
			input:    "\x1b[0m\x1b[38;2;247;250;252m\x1b[0m  \x1b[38;2;247;250;252m• Item one\x1b[0m",
			expected: "\x1b[38;2;247;250;252m• Item one\x1b[0m",
			desc:     "Mixed ANSI codes and spaces at start should be trimmed correctly",
		},
		{
			name:     "Unicode characters with leading spaces",
			input:    "  ℹ hello → world",
			expected: "ℹ hello → world",
			desc:     "Unicode characters with leading spaces should be trimmed correctly",
		},
		{
			name:     "Unicode with ANSI and leading spaces",
			input:    "\x1b[38;2;247;250;252m   \x1b[0m\x1b[38;2;247;250;252mℹ hello → world\x1b[0m",
			expected: "\x1b[0m\x1b[38;2;247;250;252mℹ hello → world\x1b[0m",
			desc:     "Unicode with ANSI codes and leading spaces should trim correctly (reset code preserved)",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			desc:     "Empty string should remain empty",
		},
		{
			name:     "only spaces",
			input:    "     ",
			expected: "",
			desc:     "String with only spaces should become empty",
		},
		{
			name:     "only ANSI wrapped spaces",
			input:    "\x1b[38;2;247;250;252m     \x1b[0m",
			expected: "",
			desc:     "String with only ANSI-wrapped spaces should become empty",
		},
		{
			name:     "preserves trailing spaces",
			input:    "   hello world   ",
			expected: "hello world   ",
			desc:     "Trailing spaces should be preserved, only leading removed",
		},
		{
			name:     "preserves ANSI on trailing spaces",
			input:    "\x1b[38;2;247;250;252m   \x1b[0m\x1b[38;2;247;250;252mhello world\x1b[0m\x1b[38;2;247;250;252m   \x1b[0m",
			expected: "\x1b[0m\x1b[38;2;247;250;252mhello world\x1b[0m\x1b[38;2;247;250;252m   \x1b[0m",
			desc:     "ANSI codes on trailing spaces should be preserved (leading reset code after trim)",
		},
		{
			name:     "real Glamour output with bullet",
			input:    "\x1b[38;2;247;250;252m\x1b[0m\x1b[38;2;247;250;252m\x1b[0m  \x1b[38;2;247;250;252m• \x1b[0m\x1b[38;2;247;250;252mItem one\x1b[0m",
			expected: "\x1b[38;2;247;250;252m• \x1b[0m\x1b[38;2;247;250;252mItem one\x1b[0m",
			desc:     "Real Glamour bullet list output should have leading spaces trimmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimLeftSpaces(tt.input)

			// Compare results.
			if result != tt.expected {
				t.Errorf("\nTest: %s\nDescription: %s\n\nInput:\n  Raw: %q\n  Hex: % X\n  Visual: %s\n\nExpected:\n  Raw: %q\n  Hex: % X\n  Visual: %s\n\nGot:\n  Raw: %q\n  Hex: % X\n  Visual: %s",
					tt.name,
					tt.desc,
					tt.input,
					[]byte(tt.input),
					tt.input,
					tt.expected,
					[]byte(tt.expected),
					tt.expected,
					result,
					[]byte(result),
					result,
				)
			}

			// Additional verification: check visual width.
			strippedInput := ansi.Strip(tt.input)
			strippedExpected := ansi.Strip(tt.expected)
			strippedResult := ansi.Strip(result)

			expectedWidth := ansi.StringWidth(strings.TrimLeft(strippedInput, " "))
			resultWidth := ansi.StringWidth(strippedResult)

			if resultWidth != expectedWidth {
				t.Errorf("\nVisual width mismatch:\n  Expected trimmed width: %d (from %q)\n  Got width: %d (from %q)",
					expectedWidth,
					strings.TrimLeft(strippedInput, " "),
					resultWidth,
					strippedResult,
				)
			}

			// Verify no leading whitespace in result.
			if strippedResult != strings.TrimLeft(strippedResult, " ") {
				t.Errorf("\nResult still has leading whitespace:\n  Stripped result: %q\n  After TrimLeft: %q",
					strippedResult,
					strings.TrimLeft(strippedResult, " "),
				)
			}

			// Verify expected also matches this property.
			if strippedExpected != strings.TrimLeft(strippedExpected, " ") {
				t.Errorf("\nTest case error - expected value has leading whitespace:\n  Stripped expected: %q\n  After TrimLeft: %q",
					strippedExpected,
					strings.TrimLeft(strippedExpected, " "),
				)
			}
		})
	}
}

func TestReset(t *testing.T) {
	// Initialize first.
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// Verify initialized.
	if Format == nil {
		t.Fatal("Format should be initialized after InitFormatter")
	}

	// Reset.
	Reset()

	// Verify reset.
	if Format != nil {
		t.Error("Format should be nil after Reset")
	}

	// Re-initialize for other tests.
	InitFormatter(ioCtx)
}

func TestSetColorProfile(t *testing.T) {
	// This function should not panic.
	SetColorProfile(termenv.Ascii)
	SetColorProfile(termenv.ANSI)
	SetColorProfile(termenv.ANSI256)
	SetColorProfile(termenv.TrueColor)
}

func TestHint(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// Hint no longer returns an error - it logs internally if there's a write failure.
	Hint("This is a hint")
}

func TestHintf(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// Hintf no longer returns an error - it logs internally if there's a write failure.
	Hintf("This is a %s hint", "formatted")
}

func TestExperimental(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// Experimental no longer returns an error - it logs internally if there's a write failure.
	Experimental("test-feature")
}

func TestExperimentalf(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// Experimentalf no longer returns an error - it logs internally if there's a write failure.
	Experimentalf("test-%s", "feature")
}

func TestBadge(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	result := Badge("TEST", "#FF9800", "#000000")
	if result == "" {
		t.Error("Badge() returned empty string")
	}
	// Badge should contain the text.
	if !strings.Contains(result, "TEST") {
		t.Errorf("Badge() should contain text 'TEST', got: %q", result)
	}
}

func TestFormatExperimentalBadge(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	result := FormatExperimentalBadge()
	if result == "" {
		t.Error("FormatExperimentalBadge() returned empty string")
	}
	// Should contain EXPERIMENTAL.
	if !strings.Contains(result, "EXPERIMENTAL") {
		t.Errorf("FormatExperimentalBadge() should contain 'EXPERIMENTAL', got: %q", result)
	}
}

func TestClearLine(t *testing.T) {
	ioCtx := createTestIOContext()
	InitFormatter(ioCtx)

	// ClearLine no longer returns an error - it logs internally if there's a write failure.
	ClearLine()
}

func TestConfigureColorProfileAllProfiles(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
	}{
		{name: "ColorNone", profile: terminal.ColorNone},
		{name: "Color16", profile: terminal.Color16},
		{name: "Color256", profile: terminal.Color256},
		{name: "ColorTrue", profile: terminal.ColorTrue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := &mockTerminal{profile: tt.profile}
			// Should not panic.
			configureColorProfile(term)
		})
	}
}

func TestFormatterHintMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	// Test the Hint method on Formatter instance.
	result := f.Hint("This is a hint message")
	if result == "" {
		t.Error("Formatter.Hint() returned empty string")
	}
}

func TestFormatterHintfMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	// Test the Hintf method on Formatter instance.
	result := f.Hintf("This is a %s hint", "formatted")
	if result == "" {
		t.Error("Formatter.Hintf() returned empty string")
	}
}

func TestFormatterToastMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	// Test the Toast method on Formatter instance.
	result := f.Toast("🎉", "Test message")
	if result == "" {
		t.Error("Formatter.Toast() returned empty string")
	}
	if !strings.Contains(result, "Test message") {
		t.Errorf("Formatter.Toast() should contain message, got: %q", result)
	}
}

func TestFormatterToastfMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	// Test the Toastf method on Formatter instance.
	result := f.Toastf("📣", "Value is %d", 42)
	if result == "" {
		t.Error("Formatter.Toastf() returned empty string")
	}
	if !strings.Contains(result, "42") {
		t.Errorf("Formatter.Toastf() should contain formatted value, got: %q", result)
	}
}

func TestFormatterHintWithBackticks(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	// The renderInlineMarkdownWithBase is called through Hint.
	// Test through the public API.
	result := f.Hint("Use `--help` for more info")
	if result == "" {
		t.Error("Formatter.Hint() with backticks returned empty string")
	}
}

func TestFormatterBoldMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	result := f.Bold("Bold text")
	if result == "" {
		t.Error("Formatter.Bold() returned empty string")
	}
	if !strings.Contains(result, "Bold text") {
		t.Errorf("Formatter.Bold() should contain text, got: %q", result)
	}
}

func TestFormatterMutedMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	result := f.Muted("Muted text")
	if result == "" {
		t.Error("Formatter.Muted() returned empty string")
	}
	if !strings.Contains(result, "Muted text") {
		t.Errorf("Formatter.Muted() should contain text, got: %q", result)
	}
}

func TestFormatterHeadingMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	result := f.Heading("Heading text")
	if result == "" {
		t.Error("Formatter.Heading() returned empty string")
	}
	if !strings.Contains(result, "Heading text") {
		t.Errorf("Formatter.Heading() should contain text, got: %q", result)
	}
}

func TestFormatterLabelMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := terminal.New()
	f := NewFormatter(ioCtx, term)

	result := f.Label("Label text")
	if result == "" {
		t.Error("Formatter.Label() returned empty string")
	}
	if !strings.Contains(result, "Label text") {
		t.Errorf("Formatter.Label() should contain text, got: %q", result)
	}
}

func TestExperimental_FormatterNotInitialized(t *testing.T) {
	// Save original state.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	oldTerminal := globalTerminal
	globalFormatter = nil
	Format = nil
	globalTerminal = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		globalTerminal = oldTerminal
		formatterMu.Unlock()
	}()

	// Experimental no longer returns errors - it logs internally.
	// This test verifies that calling the function when not initialized doesn't panic.
	Experimental("test-feature")
}

func TestExperimentalf_FormatterNotInitialized(t *testing.T) {
	// Save original state.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	oldTerminal := globalTerminal
	globalFormatter = nil
	Format = nil
	globalTerminal = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		globalTerminal = oldTerminal
		formatterMu.Unlock()
	}()

	// Experimentalf no longer returns errors - it logs internally.
	// This test verifies that calling the function when not initialized doesn't panic.
	Experimentalf("test-%s", "feature")
}

func TestBadge_FormatterNotInitialized(t *testing.T) {
	// Save original state.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	oldTerminal := globalTerminal
	globalFormatter = nil
	Format = nil
	globalTerminal = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		globalTerminal = oldTerminal
		formatterMu.Unlock()
	}()

	// Badge should return fallback format when formatter not initialized.
	result := Badge("TEST", "#FF9800", "#000000")
	expected := "[TEST]"
	if result != expected {
		t.Errorf("Badge() fallback = %q, want %q", result, expected)
	}
}

func TestFormatExperimentalBadge_FormatterNotInitialized(t *testing.T) {
	// Save original state.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	oldTerminal := globalTerminal
	globalFormatter = nil
	Format = nil
	globalTerminal = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		globalTerminal = oldTerminal
		formatterMu.Unlock()
	}()

	// FormatExperimentalBadge should return fallback format when formatter not initialized.
	result := FormatExperimentalBadge()
	expected := "[EXPERIMENTAL]"
	if result != expected {
		t.Errorf("FormatExperimentalBadge() fallback = %q, want %q", result, expected)
	}
}

func TestFormatExperimentalBadge_NoColorSupport(t *testing.T) {
	// Initialize formatter with no color support.
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	formatterMu.Lock()
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)
	Format = globalFormatter
	globalTerminal = term
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = nil
		Format = nil
		globalTerminal = nil
		formatterMu.Unlock()
	}()

	// FormatExperimentalBadge should return plain format when color not supported.
	result := FormatExperimentalBadge()
	expected := "[EXPERIMENTAL]"
	if result != expected {
		t.Errorf("FormatExperimentalBadge() with no color = %q, want %q", result, expected)
	}
}

func TestClearLine_TerminalNotInitialized(t *testing.T) {
	// Save original state.
	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldFormat := Format
	oldTerminal := globalTerminal
	globalTerminal = nil
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		Format = oldFormat
		globalTerminal = oldTerminal
		formatterMu.Unlock()
	}()

	// ClearLine no longer returns errors - it logs internally.
	// This test verifies that calling the function when not initialized doesn't panic.
	ClearLine()
}

func TestClearLine_WithColorSupport(t *testing.T) {
	// Initialize with color support.
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorTrue)
	formatterMu.Lock()
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)
	Format = globalFormatter
	globalTerminal = term
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = nil
		Format = nil
		globalTerminal = nil
		formatterMu.Unlock()
	}()

	// ClearLine no longer returns errors - it logs internally if there's a write failure.
	ClearLine()
}

func TestClearLine_NoColorSupport(t *testing.T) {
	// Initialize without color support.
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	formatterMu.Lock()
	globalFormatter = NewFormatter(ioCtx, term).(*formatter)
	Format = globalFormatter
	globalTerminal = term
	formatterMu.Unlock()

	// Restore after test.
	defer func() {
		formatterMu.Lock()
		globalFormatter = nil
		Format = nil
		globalTerminal = nil
		formatterMu.Unlock()
	}()

	// ClearLine no longer returns errors - it logs internally if there's a write failure.
	// When color is not supported, it uses \r fallback.
	ClearLine()
}

func TestFormatterExperimentalfMethod(t *testing.T) {
	ioCtx := createTestIOContext()
	term := createMockTerminal(terminal.ColorNone)
	f := NewFormatter(ioCtx, term).(*formatter)

	// Test the method on formatter struct.
	// Experimentalf formats the feature name and passes to Experimental.
	result := f.Experimentalf("%s", "toolchain")
	if result == "" {
		t.Error("Formatter.Experimentalf() returned empty string")
	}
	// The method outputs the feature name in the experimental message.
	if !strings.Contains(result, "toolchain") {
		t.Errorf("Formatter.Experimentalf() should contain feature name, got: %q", result)
	}
	if !strings.Contains(result, "experimental feature") {
		t.Errorf("Formatter.Experimentalf() should contain 'experimental feature', got: %q", result)
	}
}

func TestFormatterBadgeMethod(t *testing.T) {
	tests := []struct {
		name       string
		profile    terminal.ColorProfile
		text       string
		background string
		foreground string
	}{
		{
			name:       "Badge with color support",
			profile:    terminal.ColorTrue,
			text:       "TEST",
			background: "#FF9800",
			foreground: "#000000",
		},
		{
			name:       "Badge without color support",
			profile:    terminal.ColorNone,
			text:       "BADGE",
			background: "#0000FF",
			foreground: "#FFFFFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term).(*formatter)

			result := f.Badge(tt.text, tt.background, tt.foreground)
			if result == "" {
				t.Error("Formatter.Badge() returned empty string")
			}
			if !strings.Contains(result, tt.text) {
				t.Errorf("Formatter.Badge() should contain text %q, got: %q", tt.text, result)
			}

			// Without color, should return plain bracketed format.
			if tt.profile == terminal.ColorNone {
				expected := "[" + tt.text + "]"
				if result != expected {
					t.Errorf("Formatter.Badge() without color = %q, want %q", result, expected)
				}
			}
		})
	}
}

func TestFormatterExperimentalMethod(t *testing.T) {
	tests := []struct {
		name    string
		profile terminal.ColorProfile
		feature string
	}{
		{
			name:    "Experimental with color support",
			profile: terminal.ColorTrue,
			feature: "new-feature",
		},
		{
			name:    "Experimental without color support",
			profile: terminal.ColorNone,
			feature: "test-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContext()
			term := createMockTerminal(tt.profile)
			f := NewFormatter(ioCtx, term).(*formatter)

			result := f.Experimental(tt.feature)
			if result == "" {
				t.Error("Formatter.Experimental() returned empty string")
			}
			// The method outputs the feature name in the experimental message.
			if !strings.Contains(result, tt.feature) {
				t.Errorf("Formatter.Experimental() should contain feature name %q, got: %q", tt.feature, result)
			}
			if !strings.Contains(result, "experimental feature") {
				t.Errorf("Formatter.Experimental() should contain 'experimental feature', got: %q", result)
			}
		})
	}
}

// TestHint_PackageLevel tests the Hint package-level function.
func TestHint_PackageLevel(t *testing.T) {
	t.Run("hint when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Hint("This is a hint message")
	})

	t.Run("hint when not initialized", func(t *testing.T) {
		// Temporarily clear global formatter.
		formatterMu.Lock()
		oldFormatter := globalFormatter
		globalFormatter = nil
		formatterMu.Unlock()

		defer func() {
			formatterMu.Lock()
			globalFormatter = oldFormatter
			formatterMu.Unlock()
		}()

		// Should not panic when not initialized.
		Hint("This should not panic")
	})
}

// TestHintf_PackageLevel tests the Hintf package-level function.
func TestHintf_PackageLevel(t *testing.T) {
	t.Run("hintf when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Hintf("Use `%s` to %s", "atmos help", "get help")
	})

	t.Run("hintf when not initialized", func(t *testing.T) {
		// Temporarily clear global formatter.
		formatterMu.Lock()
		oldFormatter := globalFormatter
		globalFormatter = nil
		formatterMu.Unlock()

		defer func() {
			formatterMu.Lock()
			globalFormatter = oldFormatter
			formatterMu.Unlock()
		}()

		// Should not panic when not initialized.
		Hintf("This should not panic: %s", "test")
	})
}

// TestMarkdown_PackageLevel tests the Markdown package-level function.
func TestMarkdown_PackageLevel(t *testing.T) {
	t.Run("markdown when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Markdown("# Header\n\nThis is **bold** text.")
	})

	t.Run("markdown when not initialized", func(t *testing.T) {
		// Temporarily clear global formatter and IO.
		formatterMu.Lock()
		oldFormatter := globalFormatter
		oldIO := globalIO
		globalFormatter = nil
		globalIO = nil
		formatterMu.Unlock()

		defer func() {
			formatterMu.Lock()
			globalFormatter = oldFormatter
			globalIO = oldIO
			formatterMu.Unlock()
		}()

		// Should not panic when not initialized.
		Markdown("# This should not panic")
	})
}

// TestMarkdownf_PackageLevel tests the Markdownf package-level function.
func TestMarkdownf_PackageLevel(t *testing.T) {
	t.Run("markdownf when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Markdownf("# %s\n\nVersion: **%s**", "Atmos", "1.0.0")
	})
}

// TestMarkdownMessage_PackageLevel tests the MarkdownMessage package-level function.
func TestMarkdownMessage_PackageLevel(t *testing.T) {
	t.Run("markdown message when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		MarkdownMessage("**Error:** Something went wrong")
	})

	t.Run("markdown message when not initialized", func(t *testing.T) {
		// Temporarily clear global formatter and IO.
		formatterMu.Lock()
		oldFormatter := globalFormatter
		oldIO := globalIO
		globalFormatter = nil
		globalIO = nil
		formatterMu.Unlock()

		defer func() {
			formatterMu.Lock()
			globalFormatter = oldFormatter
			globalIO = oldIO
			formatterMu.Unlock()
		}()

		// Should not panic when not initialized.
		MarkdownMessage("**This should not panic**")
	})
}

// TestMarkdownMessagef_PackageLevel tests the MarkdownMessagef package-level function.
func TestMarkdownMessagef_PackageLevel(t *testing.T) {
	t.Run("markdown messagef when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		MarkdownMessagef("**%s:** %s", "Warning", "Check configuration")
	})
}

// TestWrite_PackageLevel tests the Write package-level function.
func TestWrite_PackageLevel(t *testing.T) {
	t.Run("write when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Write("Plain text message")
	})

	t.Run("write when not initialized", func(t *testing.T) {
		// Temporarily clear global formatter.
		formatterMu.Lock()
		oldFormatter := globalFormatter
		globalFormatter = nil
		formatterMu.Unlock()

		defer func() {
			formatterMu.Lock()
			globalFormatter = oldFormatter
			formatterMu.Unlock()
		}()

		// Should not panic when not initialized.
		Write("This should not panic")
	})
}

// TestWritef_PackageLevel tests the Writef package-level function.
func TestWritef_PackageLevel(t *testing.T) {
	t.Run("writef when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Writef("Processing %d items", 42)
	})
}

// TestWriteln_PackageLevel tests the Writeln package-level function.
func TestWriteln_PackageLevel(t *testing.T) {
	t.Run("writeln when initialized", func(t *testing.T) {
		ioCtx := createTestIOContext()
		term := createMockTerminal(terminal.ColorNone)
		InitFormatter(ioCtx)
		globalFormatter = NewFormatter(ioCtx, term).(*formatter)

		// Should not panic when initialized.
		Writeln("Line of text")
	})
}
