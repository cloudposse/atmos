package ansi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "simple color code",
			input:    "\x1b[31mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "bold text",
			input:    "\x1b[1mbold\x1b[0m",
			expected: "bold",
		},
		{
			name:     "multiple colors",
			input:    "\x1b[31mred\x1b[0m \x1b[32mgreen\x1b[0m",
			expected: "red green",
		},
		{
			name:     "256 color",
			input:    "\x1b[38;5;196mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "true color (24-bit)",
			input:    "\x1b[38;2;255;0;0mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "background color",
			input:    "\x1b[48;2;0;163;224mblue bg\x1b[0m",
			expected: "blue bg",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only ANSI codes",
			input:    "\x1b[31m\x1b[0m",
			expected: "",
		},
		{
			name:     "nested styles",
			input:    "\x1b[1m\x1b[31mbold red\x1b[0m\x1b[0m",
			expected: "bold red",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Strip(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLength(t *testing.T) {
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
			name:     "colored text",
			input:    "\x1b[31mhello\x1b[0m",
			expected: 5,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "only ANSI codes",
			input:    "\x1b[31m\x1b[0m",
			expected: 0,
		},
		{
			name:     "H1 badge with padding",
			input:    "\x1b[38;2;247;250;252;48;2;0;163;224;1m About Atmos \x1b[0m",
			expected: 13, // " About Atmos " with spaces
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Length(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsStart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pos      int
		expected bool
	}{
		{
			name:     "start of ANSI sequence",
			input:    "\x1b[31m",
			pos:      0,
			expected: true,
		},
		{
			name:     "not start position",
			input:    "a\x1b[31m",
			pos:      0,
			expected: false,
		},
		{
			name:     "ESC without bracket",
			input:    "\x1bm",
			pos:      0,
			expected: false,
		},
		{
			name:     "at end of string",
			input:    "a\x1b",
			pos:      1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStart(tt.input, tt.pos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSkip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pos      int
		expected int
	}{
		{
			name:     "simple color",
			input:    "\x1b[31mtext",
			pos:      0,
			expected: 5, // After 'm'
		},
		{
			name:     "reset code",
			input:    "\x1b[0mtext",
			pos:      0,
			expected: 4, // After 'm'
		},
		{
			name:     "true color",
			input:    "\x1b[38;2;255;0;0mtext",
			pos:      0,
			expected: 15, // After 'm' (ESC=1 + [=1 + 38;2;255;0;0=12 + m=1 = 15)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skip(tt.input, tt.pos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindLastEnd(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "no ANSI codes",
			input:    "hello",
			expected: -1,
		},
		{
			name:     "one ANSI code at start",
			input:    "\x1b[31mhello",
			expected: 5,
		},
		{
			name:     "ANSI code at end",
			input:    "hello\x1b[0m",
			expected: 9,
		},
		{
			name:     "multiple ANSI codes",
			input:    "\x1b[31mhello\x1b[0m world\x1b[32m!",
			expected: 25, // Last ANSI code \x1b[32m ends at position 24, so 25 is after it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastEnd(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
