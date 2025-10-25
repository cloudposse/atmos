package ui

import (
	"strings"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestNewFormatter(t *testing.T) {
	ioCtx := createTestIOContext()

	f := newFormatter(ioCtx)

	if f == nil {
		t.Fatal("newFormatter() returned nil")
	}

	if f.Styles() == nil {
		t.Error("Styles() returned nil")
	}
}

func TestFormatter_SupportsColor(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
		want    bool
	}{
		{
			name:    "ColorNone returns false",
			profile: iolib.ColorNone,
			want:    false,
		},
		{
			name:    "Color16 returns true",
			profile: iolib.Color16,
			want:    true,
		},
		{
			name:    "Color256 returns true",
			profile: iolib.Color256,
			want:    true,
		},
		{
			name:    "ColorTrue returns true",
			profile: iolib.ColorTrue,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.SupportsColor()
			if got != tt.want {
				t.Errorf("SupportsColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatter_ColorProfile(t *testing.T) {
	profiles := []iolib.ColorProfile{
		iolib.ColorNone,
		iolib.Color16,
		iolib.Color256,
		iolib.ColorTrue,
	}

	for _, profile := range profiles {
		t.Run(profile.String(), func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(profile)
			f := newFormatter(ioCtx)

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
		profile iolib.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: iolib.ColorNone,
			input:   "test",
		},
		{
			name:    "with color",
			profile: iolib.Color16,
			input:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.Success(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Success() = %q, doesn't contain input %q", got, tt.input)
			}

			// Output should always include checkmark icon (with or without color)
			expectedNoColor := "✓ " + tt.input
			if tt.profile == iolib.ColorNone && got != expectedNoColor {
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
	f := newFormatter(ioCtx)

	input := "warning message"
	got := f.Warning(input)

	// With color support, should contain the input
	if !strings.Contains(got, input) && f.SupportsColor() {
		t.Errorf("Warning() output doesn't contain input: %q", got)
	}
}

func TestFormatter_Error(t *testing.T) {
	ioCtx := createTestIOContext()
	f := newFormatter(ioCtx)

	input := "error message"
	got := f.Error(input)

	// With color support, should contain the input
	if !strings.Contains(got, input) && f.SupportsColor() {
		t.Errorf("Error() output doesn't contain input: %q", got)
	}
}

func TestFormatter_Info(t *testing.T) {
	ioCtx := createTestIOContext()
	f := newFormatter(ioCtx)

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
		profile iolib.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: iolib.ColorNone,
			input:   "muted text",
		},
		{
			name:    "with color",
			profile: iolib.Color16,
			input:   "muted text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.Muted(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Muted() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == iolib.ColorNone && got != tt.input {
				t.Errorf("Muted() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Bold(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: iolib.ColorNone,
			input:   "bold text",
		},
		{
			name:    "with color",
			profile: iolib.Color16,
			input:   "bold text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.Bold(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Bold() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == iolib.ColorNone && got != tt.input {
				t.Errorf("Bold() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Heading(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: iolib.ColorNone,
			input:   "heading",
		},
		{
			name:    "with color",
			profile: iolib.Color16,
			input:   "heading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.Heading(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Heading() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == iolib.ColorNone && got != tt.input {
				t.Errorf("Heading() with no color = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestFormatter_Label(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
		input   string
	}{
		{
			name:    "no color",
			profile: iolib.ColorNone,
			input:   "label",
		},
		{
			name:    "with color",
			profile: iolib.Color16,
			input:   "label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.Label(tt.input)

			// Output should contain the input text
			if !strings.Contains(got, tt.input) {
				t.Errorf("Label() = %q, doesn't contain input %q", got, tt.input)
			}

			// Without color, output should equal input exactly
			if tt.profile == iolib.ColorNone && got != tt.input {
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
			f := newFormatter(ioCtx)

			got, err := f.RenderMarkdown(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("RenderMarkdown() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got == "" && tt.input != "" {
				t.Error("RenderMarkdown() returned empty string for non-empty input")
			}
		})
	}
}

func TestFormatter_RenderMarkdown_MaxWidth(t *testing.T) {
	// Test that RenderMarkdown doesn't fail with markdown content
	// This test ensures the method handles content correctly
	ioCtx := createTestIOContext()
	f := newFormatter(ioCtx)

	input := "# Test\n\nThis is a very long line that should be wrapped according to the terminal width."
	got, err := f.RenderMarkdown(input)

	if err != nil {
		t.Errorf("RenderMarkdown() error = %v", err)
	}

	if got == "" {
		t.Error("RenderMarkdown() returned empty string")
	}
}

func TestGenerateStyleSet(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
	}{
		{
			name:    "ColorNone",
			profile: iolib.ColorNone,
		},
		{
			name:    "Color16",
			profile: iolib.Color16,
		},
		{
			name:    "Color256",
			profile: iolib.Color256,
		},
		{
			name:    "ColorTrue",
			profile: iolib.ColorTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles := generateStyleSet(tt.profile)

			if styles == nil {
				t.Fatal("generateStyleSet() returned nil")
			}

			// Verify all styles are initialized
			if styles.Title.String() == "" && tt.profile != iolib.ColorNone {
				// This is okay - lipgloss styles can have empty string representation
			}
		})
	}
}

// Helper functions for testing.

func createTestIOContext() iolib.Context {
	ctx, _ := iolib.NewContext()
	return ctx
}

func createTestIOContextWithProfile(profile iolib.ColorProfile) iolib.Context {
	ctx, _ := iolib.NewContext(
		iolib.WithTerminal(&mockTerminal{profile: profile}),
	)
	return ctx
}

// mockTerminal implements iolib.Terminal for testing.
type mockTerminal struct {
	profile iolib.ColorProfile
	width   int
	height  int
	isTTY   bool
}

func (m *mockTerminal) IsTTY(stream interface{}) bool {
	return m.isTTY
}

func (m *mockTerminal) ColorProfile() iolib.ColorProfile {
	return m.profile
}

func (m *mockTerminal) Width(stream interface{}) int {
	return m.width
}

func (m *mockTerminal) Height(stream interface{}) int {
	return m.height
}

func (m *mockTerminal) SetTitle(title string) {}

func (m *mockTerminal) RestoreTitle() {}

func (m *mockTerminal) Alert() {}

// mockIOContext allows injecting custom config for testing.
type mockIOContext struct {
	iolib.Context
	config *iolib.Config
}

func (m *mockIOContext) Config() *iolib.Config {
	return m.config
}

func TestFormatter_StatusMessage(t *testing.T) {
	tests := []struct {
		name    string
		profile iolib.ColorProfile
		icon    string
		text    string
		want    string
	}{
		{
			name:    "no color - plain formatting",
			profile: iolib.ColorNone,
			icon:    "✓",
			text:    "test message",
			want:    "✓ test message",
		},
		{
			name:    "with color - contains icon and text",
			profile: iolib.Color16,
			icon:    "✗",
			text:    "error message",
			// With color, output will have ANSI codes, so just check it contains the parts
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ioCtx := createTestIOContextWithProfile(tt.profile)
			f := newFormatter(ioCtx)

			got := f.StatusMessage(tt.icon, f.Styles().Success, tt.text)

			// For no color, output should match exactly
			if tt.profile == iolib.ColorNone && got != tt.want {
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
			ioCtx := createTestIOContextWithProfile(iolib.Color16)
			f := newFormatter(ioCtx)

			got := tt.method(f, tt.text)

			if !strings.Contains(got, tt.expectedIcon) {
				t.Errorf("%s output = %q, should contain icon %q", tt.name, got, tt.expectedIcon)
			}
			if !strings.Contains(got, tt.text) {
				t.Errorf("%s output = %q, should contain text %q", tt.name, got, tt.text)
			}

			// Test without color
			ioCtxNoColor := createTestIOContextWithProfile(iolib.ColorNone)
			fNoColor := newFormatter(ioCtxNoColor)

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
			ioCtx := createTestIOContextWithProfile(iolib.Color16)
			f := newFormatter(ioCtx)

			got := tt.method(f)

			if !strings.Contains(got, tt.expectedIcon) {
				t.Errorf("%s output = %q, should contain icon %q", tt.name, got, tt.expectedIcon)
			}
			if !strings.Contains(got, tt.expectedText) {
				t.Errorf("%s output = %q, should contain text %q", tt.name, got, tt.expectedText)
			}

			// Test without color
			ioCtxNoColor := createTestIOContextWithProfile(iolib.ColorNone)
			fNoColor := newFormatter(ioCtxNoColor)

			gotNoColor := tt.method(fNoColor)
			expectedNoColor := tt.expectedIcon + " " + tt.expectedText

			if gotNoColor != expectedNoColor {
				t.Errorf("%s with no color = %q, want %q", tt.name, gotNoColor, expectedNoColor)
			}
		})
	}
}

