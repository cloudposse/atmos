package ansi

import (
	"strings"
	"testing"

	externalansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestTrimRight(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		expected            string
		desc                string
		preservesStyledText bool // true if styled suffix space is intentionally preserved
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
		{
			name:                "H1 header badge with styled suffix space",
			input:               "\x1b[38;2;247;250;252;48;2;0;163;224;1m About Atmos \x1b[0m",
			expected:            "\x1b[38;2;247;250;252;48;2;0;163;224;1m About Atmos \x1b[0m",
			desc:                "H1 badge with background color (48;2;...) should preserve trailing space as styled content",
			preservesStyledText: true, // Trailing space is part of styled badge content
		},
		{
			name:                "H1 header badge followed by Glamour padding",
			input:               "\x1b[38;2;247;250;252;48;2;0;163;224;1m About Atmos \x1b[0m\x1b[38;2;247;250;252m     \x1b[0m",
			expected:            "\x1b[38;2;247;250;252;48;2;0;163;224;1m About Atmos \x1b[0m",
			desc:                "H1 badge should preserve styled suffix but trim following Glamour padding",
			preservesStyledText: true, // Trailing space is part of styled badge content
		},
		// Edge cases for loop iteration coverage.
		{
			name:     "consecutive resets then styled space",
			input:    "hello\x1b[0m\x1b[0m\x1b[38;2;100;100;100m \x1b[0m",
			expected: "hello\x1b[0m",
			desc:     "Tests loop iteration: trimConsecutiveResets then trimTrailingStyledSpace",
		},
		{
			name:     "styled space then bare ANSI",
			input:    "hello\x1b[38;2;100;100;100m \x1b[0m\x1b[38;2;100;100;100m",
			expected: "hello",
			desc:     "Tests loop iteration: trimTrailingStyledSpace then trimTrailingBareANSI",
		},
		{
			name:     "multiple styled spaces in sequence",
			input:    "hello\x1b[38;2;100;100;100m \x1b[0m\x1b[38;2;100;100;100m \x1b[0m\x1b[38;2;100;100;100m \x1b[0m",
			expected: "hello",
			desc:     "Tests multiple iterations of trimTrailingStyledSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimRight(tt.input)

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

			// Additional verification for cases that don't preserve styled text.
			// When preservesStyledText is true, trailing whitespace is intentional (e.g., H1 badges).
			if !tt.preservesStyledText {
				strippedInput := Strip(tt.input)
				strippedExpected := Strip(tt.expected)
				strippedResult := Strip(result)

				// Use external ansi package for Unicode-aware string width.
				expectedWidth := externalansi.StringWidth(strings.TrimRight(strippedInput, " \t"))
				resultWidth := externalansi.StringWidth(strippedResult)

				if resultWidth != expectedWidth {
					t.Errorf("\nVisual width mismatch:\n  Expected trimmed width: %d (from %q)\n  Got width: %d (from %q)",
						expectedWidth,
						strings.TrimRight(strippedInput, " \t"),
						resultWidth,
						strippedResult,
					)
				}

				// Verify no trailing whitespace in result.
				if strippedResult != strings.TrimRight(strippedResult, " \t") {
					t.Errorf("\nResult still has trailing whitespace:\n  Stripped result: %q\n  After TrimRight: %q",
						strippedResult,
						strings.TrimRight(strippedResult, " \t"),
					)
				}

				// Verify expected also matches this property.
				if strippedExpected != strings.TrimRight(strippedExpected, " \t") {
					t.Errorf("\nTest case error - expected value has trailing whitespace:\n  Stripped expected: %q\n  After TrimRight: %q",
						strippedExpected,
						strings.TrimRight(strippedExpected, " \t"),
					)
				}
			}
		})
	}
}

func TestTrimLinesRight(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line no change",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "single line with trailing spaces",
			input:    "hello world   ",
			expected: "hello world",
		},
		{
			name:     "multiple lines with trailing spaces",
			input:    "line one   \nline two  \nline three    ",
			expected: "line one\nline two\nline three",
		},
		{
			name:     "multiple lines with ANSI",
			input:    "\x1b[31mred\x1b[0m   \n\x1b[32mgreen\x1b[0m  ",
			expected: "\x1b[31mred\x1b[0m\n\x1b[32mgreen\x1b[0m",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only newlines",
			input:    "\n\n\n",
			expected: "\n\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimLinesRight(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
		// Edge case: non-space content encountered during skip loop (break skipLoop).
		{
			name:     "non-space found during skip loop",
			input:    "  X  hello",
			expected: "X  hello",
			desc:     "Non-space 'X' found during skip loop triggers break",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimLeftSpaces(tt.input)

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
			strippedInput := Strip(tt.input)
			strippedExpected := Strip(tt.expected)
			strippedResult := Strip(result)

			// Use external ansi package for Unicode-aware string width.
			expectedWidth := externalansi.StringWidth(strings.TrimLeft(strippedInput, " "))
			resultWidth := externalansi.StringWidth(strippedResult)

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

func TestTrimRightSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text no trailing spaces",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "plain text with trailing spaces",
			input:    "hello world   ",
			expected: "hello world",
		},
		{
			name:     "ANSI colored with trailing spaces",
			input:    "\x1b[31mhello\x1b[0m   ",
			expected: "\x1b[31mhello\x1b[0m",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "     ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimRightSpaces(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimTrailingWhitespace(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		indent        string
		indentWidth   int
		expectedLines []string
	}{
		{
			name:          "single line no whitespace",
			input:         "hello world",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{"hello world"},
		},
		{
			name:          "single line with trailing spaces",
			input:         "hello world   ",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{"hello world"},
		},
		{
			name:          "multiple lines",
			input:         "line one   \nline two  \nline three",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{"line one", "line two", "line three"},
		},
		{
			name:          "empty line preserves indent",
			input:         "line one\n     \nline three",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{"line one", "  ", "line three"},
		},
		// Edge case: fewer spaces than indent width.
		{
			name:          "empty line with one space less than indent width",
			input:         "line one\n \nline three",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{"line one", " ", "line three"},
		},
		{
			name:          "empty string",
			input:         "",
			indent:        "  ",
			indentWidth:   2,
			expectedLines: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimTrailingWhitespace(tt.input, tt.indent, tt.indentWidth)
			assert.Equal(t, tt.expectedLines, result)
		})
	}
}

func TestTrimConsecutiveResets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no resets",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "single reset at end",
			input:    "hello\x1b[0m",
			expected: "hello\x1b[0m",
		},
		{
			name:     "double reset at end",
			input:    "hello\x1b[0m\x1b[0m",
			expected: "hello\x1b[0m",
		},
		{
			name:     "triple reset at end",
			input:    "hello\x1b[0m\x1b[0m\x1b[0m",
			expected: "hello\x1b[0m\x1b[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimConsecutiveResets(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimPlainTrailing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no trailing whitespace",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "trailing spaces",
			input:    "hello world   ",
			expected: "hello world",
		},
		{
			name:     "trailing spaces after ANSI",
			input:    "\x1b[31mhello\x1b[0m   ",
			expected: "\x1b[31mhello\x1b[0m",
		},
		{
			name:     "ANSI at end no trailing",
			input:    "\x1b[31mhello\x1b[0m",
			expected: "\x1b[31mhello\x1b[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimPlainTrailing(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimTrailingStyledSpace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no styled space",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "foreground-only styled space - trim",
			input:    "hello\x1b[38;2;247;250;252m \x1b[0m",
			expected: "hello",
		},
		{
			name:     "background styled space - preserve",
			input:    "hello\x1b[48;2;0;163;224m \x1b[0m",
			expected: "hello\x1b[48;2;0;163;224m \x1b[0m",
		},
		{
			name:     "combined fg+bg styled space - preserve",
			input:    "hello\x1b[38;2;247;250;252;48;2;0;163;224m \x1b[0m",
			expected: "hello\x1b[38;2;247;250;252;48;2;0;163;224m \x1b[0m",
		},
		// Edge cases for early return coverage.
		{
			name:     "just reset code",
			input:    "\x1b[0m",
			expected: "\x1b[0m",
		},
		{
			name:     "non-space before reset",
			input:    "hello\x1b[38;2;247;250;252mX\x1b[0m",
			expected: "hello\x1b[38;2;247;250;252mX\x1b[0m",
		},
		{
			name:     "plain space before reset no ANSI",
			input:    " \x1b[0m",
			expected: " \x1b[0m",
		},
		{
			name:     "malformed ANSI no m terminator",
			input:    "hello\x1b[38;2;247 \x1b[0m",
			expected: "hello\x1b[38;2;247 \x1b[0m",
		},
		{
			name:     "does not end with reset",
			input:    "hello world",
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimTrailingStyledSpace(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimTrailingBareANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "ANSI with content after",
			input:    "\x1b[31mhello\x1b[0m world",
			expected: "\x1b[31mhello\x1b[0m world",
		},
		{
			name:     "reset code at end - preserve",
			input:    "hello\x1b[0m",
			expected: "hello\x1b[0m",
		},
		{
			name:     "color code at end - trim",
			input:    "hello\x1b[31m",
			expected: "hello",
		},
		{
			name:     "complex color at end - trim",
			input:    "hello\x1b[38;2;247;250;252m",
			expected: "hello",
		},
		// Edge cases for coverage.
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "malformed ANSI no terminator",
			input:    "hello\x1b[38;2;247",
			expected: "hello\x1b[38;2;247",
		},
		{
			name:     "ANSI sequence in middle",
			input:    "\x1b[31mhello\x1b[0m world",
			expected: "\x1b[31mhello\x1b[0m world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimTrailingBareANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
