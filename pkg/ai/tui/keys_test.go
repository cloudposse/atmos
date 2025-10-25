package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no ANSI sequences",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "CSI color sequence",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "cursor position report",
			input:    "text\x1b[24;80Rmore text",
			expected: "textmore text",
		},
		{
			name:     "bare cursor position fragment at start",
			input:    "24Rmore",
			expected: "more",
		},
		{
			name:     "OSC sequence with BEL terminator",
			input:    "text\x1b]0;window title\x07more",
			expected: "textmore",
		},
		{
			name:     "OSC sequence with ST terminator",
			input:    "text\x1b]11;rgb:0000/0000/0000\x1b\\more",
			expected: "textmore",
		},
		{
			name:     "three-component color fragment with backslash",
			input:    "text0000/0000/0000\\more",
			expected: "textmore",
		},
		{
			name:     "three-component color fragment without backslash",
			input:    "text0000/0000/0000more",
			expected: "textmore",
		},
		{
			name:     "two-component color fragment (new pattern)",
			input:    "text000/0000\\more",
			expected: "textmore",
		},
		{
			name:     "two-component color fragment at start",
			input:    "000/0000\\hello",
			expected: "hello",
		},
		{
			name:     "mixed ANSI sequences",
			input:    "\x1b[31m000/0000\\text\x1b[0m",
			expected: "text",
		},
		{
			name:     "rgb color query fragment",
			input:    "text:0000/0000/0000\\amore",
			expected: "textmore",
		},
		{
			name:     "bare OSC fragment",
			input:    "text]11;rgb:0000/0000/0000\\more",
			expected: "textmore",
		},
		{
			name:     "hex digits 1-4 components each (new pattern variation)",
			input:    "text0/00\\more",
			expected: "textmore",
		},
		{
			name:     "hex digits different lengths (new pattern)",
			input:    "text1a2/3b4c\\more",
			expected: "textmore",
		},
		{
			name:     "real-world terminal OSC sequence",
			input:    "hello]11;rgb:0000/0000/0000\\world",
			expected: "helloworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			assert.Equal(t, tt.expected, result, "stripANSI(%q) should return %q, got %q", tt.input, tt.expected, result)
		})
	}
}

func TestStripANSI_PreservesNormalText(t *testing.T) {
	normalTexts := []string{
		"simple text",
		"text with numbers 123",
		"text/with/slashes",
		"text\\with\\backslashes (but not ANSI pattern)",
		"rgb is just text here",
		"0000 alone is fine",
		"text with spaces",
		"multi\nline\ntext",
		"tabs\there",
		"special chars: !@#$%^&*()",
	}

	for _, text := range normalTexts {
		t.Run(text, func(t *testing.T) {
			result := stripANSI(text)
			assert.Equal(t, text, result, "stripANSI should not modify normal text")
		})
	}
}

func TestANSIEscapeRegex_Compilation(t *testing.T) {
	// Verify the regex compiles without panic.
	assert.NotNil(t, ansiEscapeRegex, "ansiEscapeRegex should be compiled")
}
