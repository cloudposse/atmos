package provenance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWrapLine_WhitespaceFreeLines tests that lines without whitespace are hard-wrapped at maxWidth.
func TestWrapLine_WhitespaceFreeLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected []string
	}{
		{
			name:     "whitespace-free line exceeding maxWidth",
			input:    "aaaaaaaaaaaa",
			maxWidth: 8,
			expected: []string{"aaaaaaaa", "aaaa"},
		},
		{
			name:     "exact maxWidth without whitespace",
			input:    "aaaaaaaa",
			maxWidth: 8,
			expected: []string{"aaaaaaaa"},
		},
		{
			name:     "shorter than maxWidth",
			input:    "aaaa",
			maxWidth: 8,
			expected: []string{"aaaa"},
		},
		{
			name:     "long line gets chunked at maxWidth",
			input:    "abcdefghijklmnopqrstuvwxyz",
			maxWidth: 10,
			expected: []string{"abcdefghij", "klmnopqrst", "uvwxyz"},
		},
		{
			name:     "empty string",
			input:    "",
			maxWidth: 8,
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapLine(tt.input, tt.maxWidth)
			assert.Equal(t, tt.expected, result, "wrapLine(%q, %d)", tt.input, tt.maxWidth)

			// Verify no chunk exceeds maxWidth
			for i, chunk := range result {
				plainChunk := stripANSI(chunk)
				assert.LessOrEqual(t, len(plainChunk), tt.maxWidth,
					"Chunk %d (%q) exceeds maxWidth %d", i, chunk, tt.maxWidth)
			}
		})
	}
}

// TestWrapLine_WithWhitespace tests that lines wrap at maxWidth, consuming whitespace when it's the break point.
func TestWrapLine_WithWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected []string
	}{
		{
			name:     "break at space",
			input:    "hello world test",
			maxWidth: 11,
			expected: []string{"hello world", "test"},
		},
		{
			name:     "wraps at maxWidth even within words",
			input:    "one two three four",
			maxWidth: 10,
			expected: []string{"one two th", "ree four"},
		},
		{
			name:     "tab preserved when not at break point",
			input:    "hello\tworld",
			maxWidth: 6,
			expected: []string{"hello\t", "world"},
		},
		{
			name:     "space consumed at break point",
			input:    "hello world",
			maxWidth: 5,
			expected: []string{"hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapLine(tt.input, tt.maxWidth)
			assert.Equal(t, tt.expected, result, "wrapLine(%q, %d)", tt.input, tt.maxWidth)

			// Verify no chunk exceeds maxWidth
			for i, chunk := range result {
				plainChunk := stripANSI(chunk)
				assert.LessOrEqual(t, len(plainChunk), tt.maxWidth,
					"Chunk %d (%q) exceeds maxWidth %d", i, chunk, tt.maxWidth)
			}
		})
	}
}

// TestWrapLine_WithANSI tests that ANSI codes are preserved and don't count toward width.
func TestWrapLine_WithANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		verify   func(t *testing.T, result []string)
	}{
		{
			name:     "ANSI colored text respects maxWidth",
			input:    "\x1b[32mhelloworld\x1b[0m",
			maxWidth: 5,
			verify: func(t *testing.T, result []string) {
				// Should wrap at 5 visible chars, preserving ANSI codes
				assert.Len(t, result, 2, "Should wrap into 2 chunks")
				for i, chunk := range result {
					plainChunk := stripANSI(chunk)
					assert.LessOrEqual(t, len(plainChunk), 5,
						"Chunk %d plain text exceeds maxWidth", i)
					// Verify ANSI codes are preserved
					assert.Contains(t, chunk, "\x1b[", "ANSI codes should be preserved")
				}
			},
		},
		{
			name:     "ANSI codes don't count toward width",
			input:    "\x1b[31mred\x1b[0m text",
			maxWidth: 8,
			verify: func(t *testing.T, result []string) {
				// Plain text is "red text" (8 chars), should fit in one line
				assert.Len(t, result, 1, "Should not wrap - plain text is exactly 8 chars")
			},
		},
		{
			name:     "ANSI sequences kept intact during fallback hard-wrap",
			input:    "\x1b[31mlongwordwithnospacesandcolor\x1b[0m",
			maxWidth: 10,
			verify: func(t *testing.T, result []string) {
				// Should wrap into 3 chunks: 10 + 10 + 8 visible chars
				assert.Len(t, result, 3, "Should wrap into 3 chunks")

				// First chunk should have opening ANSI code and 10 chars
				assert.Contains(t, result[0], "\x1b[31m", "First chunk should have opening ANSI code")
				assert.Equal(t, 10, len(stripANSI(result[0])), "First chunk should have 10 visible chars")

				// Verify none of the chunks are just ANSI codes without text
				for i, chunk := range result {
					plainChunk := stripANSI(chunk)
					if len(plainChunk) == 0 {
						t.Errorf("Chunk %d is only ANSI codes with no visible text: %q", i, chunk)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapLine(tt.input, tt.maxWidth)
			tt.verify(t, result)
		})
	}
}

// TestStripANSI tests ANSI escape code removal.
func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple ANSI color",
			input:    "\x1b[32mgreen\x1b[0m",
			expected: "green",
		},
		{
			name:     "multiple ANSI codes",
			input:    "\x1b[31mred\x1b[0m and \x1b[34mblue\x1b[0m",
			expected: "red and blue",
		},
		{
			name:     "no ANSI codes",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCombineSideBySide tests the side-by-side layout function.
func TestCombineSideBySide(t *testing.T) {
	tests := []struct {
		name      string
		left      string
		right     string
		leftWidth int
		verify    func(t *testing.T, result string)
	}{
		{
			name:      "basic side-by-side",
			left:      "line1\nline2",
			right:     "prov1\nprov2",
			leftWidth: 20,
			verify: func(t *testing.T, result string) {
				assert.Contains(t, result, "Configuration")
				assert.Contains(t, result, "Provenance")
				assert.Contains(t, result, "│")
				assert.Contains(t, result, "line1")
				assert.Contains(t, result, "line2")
				assert.Contains(t, result, "prov1")
				assert.Contains(t, result, "prov2")
			},
		},
		{
			name:      "left longer than right",
			left:      "line1\nline2\nline3",
			right:     "prov1",
			leftWidth: 20,
			verify: func(t *testing.T, result string) {
				lines := strings.Split(result, "\n")
				// Should have header + separator + at least 3 data lines
				assert.GreaterOrEqual(t, len(lines), 5)
			},
		},
		{
			name:      "long left line wraps",
			left:      "this is a very long line that should wrap",
			right:     "prov",
			leftWidth: 50,
			verify: func(t *testing.T, result string) {
				// Verify wrapping occurred - should have multiple lines
				lines := strings.Split(result, "\n")
				assert.Greater(t, len(lines), 3, "Long line should cause wrapping")

				// The key test: verify that wrapLine was called and produced multiple chunks
				// We don't need to verify exact positioning, just that wrapping occurred
				assert.Contains(t, result, "│", "Should have separator")
				assert.Contains(t, result, "Configuration", "Should have header")
			},
		},
		{
			name:      "small leftWidth does not panic",
			left:      "test",
			right:     "prov",
			leftWidth: 5,
			verify: func(t *testing.T, result string) {
				// Should not panic even with leftWidth < 13
				assert.Contains(t, result, "Configuration", "Should have header")
				assert.Contains(t, result, "│", "Should have separator")
				// Verify it doesn't panic - if we got here, test passed
			},
		},
		{
			name:      "zero leftWidth does not panic",
			left:      "test",
			right:     "prov",
			leftWidth: 0,
			verify: func(t *testing.T, result string) {
				// Should not panic with zero leftWidth
				assert.Contains(t, result, "Configuration", "Should have header")
				// Just verify we didn't panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineSideBySide(tt.left, tt.right, tt.leftWidth)
			tt.verify(t, result)
		})
	}
}

// TestBalanceColumns tests column alignment logic.
func TestBalanceColumns(t *testing.T) {
	tests := []struct {
		name        string
		left        []string
		right       []string
		expectLeft  []string
		expectRight []string
	}{
		{
			name:        "equal length",
			left:        []string{"a", "b"},
			right:       []string{"1", "2"},
			expectLeft:  []string{"a", "b"},
			expectRight: []string{"1", "2"},
		},
		{
			name:        "left longer",
			left:        []string{"a", "b", "c"},
			right:       []string{"1"},
			expectLeft:  []string{"a", "b", "c"},
			expectRight: []string{"1", "", ""},
		},
		{
			name:        "right longer",
			left:        []string{"a"},
			right:       []string{"1", "2", "3"},
			expectLeft:  []string{"a", "", ""},
			expectRight: []string{"1", "2", "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balancedLeft, balancedRight := balanceColumns(tt.left, tt.right)
			assert.Equal(t, tt.expectLeft, balancedLeft)
			assert.Equal(t, tt.expectRight, balancedRight)
			assert.Equal(t, len(balancedLeft), len(balancedRight),
				"Balanced columns should have equal length")
		})
	}
}
