package pager

import (
	"errors"
	"strings"
	"testing"
)

func TestGetTerminalSize(t *testing.T) {
	// Save original sizer
	originalSizer := terminalSizer
	defer func() { terminalSizer = originalSizer }()

	t.Run("successful size retrieval", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 24}

		size, err := getTerminalSize()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if size.Width != 80 {
			t.Errorf("Expected width 80, got %d", size.Width)
		}
		if size.Height != 24 {
			t.Errorf("Expected height 24, got %d", size.Height)
		}
	})

	t.Run("error from terminal size call", func(t *testing.T) {
		expectedErr := errors.New("terminal size error")
		terminalSizer = MockTerminalSizer{Error: expectedErr}

		_, err := getTerminalSize()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("large terminal dimensions", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 65535, Height: 65535}

		size, err := getTerminalSize()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if size.Width != 65535 {
			t.Errorf("Expected width 65535, got %d", size.Width)
		}
		if size.Height != 65535 {
			t.Errorf("Expected height 65535, got %d", size.Height)
		}
	})
}

func TestContentFitsTerminal(t *testing.T) {
	// Save original sizer
	originalSizer := terminalSizer
	defer func() { terminalSizer = originalSizer }()

	t.Run("content fits - simple case", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 24}
		content := "Hello, World!"

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected content to fit")
		}
	})

	t.Run("content doesn't fit - too wide", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 10, Height: 24}
		content := "This is a very long line that exceeds the terminal width"

		result := ContentFitsTerminal(content)

		if result {
			t.Error("Expected content not to fit")
		}
	})

	t.Run("content doesn't fit - too tall", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 3}
		content := "Line 1\nLine 2\nLine 3\nLine 4"

		result := ContentFitsTerminal(content)

		if result {
			t.Error("Expected content not to fit")
		}
	})

	t.Run("content fits exactly", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 10, Height: 2}
		content := "1234567890\n1234567890"

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected content to fit exactly")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 24}
		content := ""

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected empty content to fit")
		}
	})

	t.Run("single newline", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 24}
		content := "\n"

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected single newline to fit")
		}
	})

	t.Run("terminal size error", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Error: errors.New("terminal error")}
		content := "Some content"

		result := ContentFitsTerminal(content)

		if result {
			t.Error("Expected content not to fit when terminal size unavailable")
		}
	})

	t.Run("content with tabs", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 20, Height: 24}
		content := "a\tb\tc" // Should expand to width 17 (a + 7 spaces + b + 7 spaces + c)

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected content with tabs to fit")
		}
	})

	t.Run("content with ANSI sequences", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 10, Height: 24}
		content := "\033[31mHello\033[0m" // Red "Hello" - should be width 5

		result := ContentFitsTerminal(content)

		if !result {
			t.Error("Expected content with ANSI sequences to fit")
		}
	})
}

func TestGetDisplayWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"simple text", "Hello", 5},
		{"single tab", "\t", 8},
		{"tab after text", "a\t", 8},
		{"tab in middle", "ab\tcd", 10},
		{"multiple tabs", "\t\t", 16},
		{"carriage return", "Hello\rWorld", 5},
		{"carriage return resets", "Hello\r", 0},
		{"printable ASCII", "ABC123!@#", 9},
		{"control characters", "\x01\x02\x03", 0},
		{"unicode characters", "héllo", 5},
		{"mixed unicode", "café", 4},
		{"ANSI color sequence", "\033[31mRed\033[0m", 3},
		{"ANSI with parameters", "\033[1;31mBold Red\033[0m", 8},
		{"ANSI CSI sequence", "\033[2J", 0},
		{"ANSI character set", "\033(B", 0},
		{"ANSI other escape", "\033c", 0},
		{"complex ANSI", "\033[?25l\033[2J\033[HHello", 5},
		{"tab alignment", "12345\t", 8},
		{"tab alignment 2", "1234567\t", 8},
		{"tab alignment 3", "12345678\t", 16},
		{"multiple lines with CR", "Hello\rWorld\rTest", 4},
		{"mixed content", "a\tb\033[31mc\033[0m\td", 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDisplayWidth(tt.input)
			if result != tt.expected {
				t.Errorf("getDisplayWidth(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSkipAnsiSequence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		expected int
	}{
		{"not ESC character", "Hello", 0, 1},
		{"ESC at end", "\033", 0, 1},
		{"simple CSI sequence", "\033[H", 0, 3},
		{"CSI with parameters", "\033[2J", 0, 4},
		{"CSI with multiple params", "\033[1;31m", 0, 7},
		{"CSI with question mark", "\033[?25l", 0, 6},
		{"CSI with exclamation", "\033[!p", 0, 4},
		{"character set G0", "\033(B", 0, 3},
		{"character set G1", "\033)B", 0, 3},
		{"other escape sequence", "\033c", 0, 2},
		{"complex CSI", "\033[38;5;196m", 0, 11},
		{"start beyond bounds", "abc", 5, 6},
		{"ESC without bracket", "\033X", 0, 2},
		{"incomplete sequence", "\033[", 0, 2},
		{"CSI with spaces", "\033[ 1 m", 0, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.input)
			result := skipAnsiSequence(runes, tt.start)
			if result != tt.expected {
				t.Errorf("skipAnsiSequence(%q, %d) = %d, expected %d", tt.input, tt.start, result, tt.expected)
			}
		})
	}
}

func TestSkipAnsiSequenceEdgeCases(t *testing.T) {
	t.Run("empty runes slice", func(t *testing.T) {
		runes := []rune{}
		result := skipAnsiSequence(runes, 0)
		expected := 1
		if result != expected {
			t.Errorf("Expected %d, got %d", expected, result)
		}
	})

	t.Run("start at exact length", func(t *testing.T) {
		runes := []rune("abc")
		result := skipAnsiSequence(runes, 3)
		expected := 4
		if result != expected {
			t.Errorf("Expected %d, got %d", expected, result)
		}
	})

	t.Run("CSI sequence at end of string", func(t *testing.T) {
		runes := []rune("text\033[")
		result := skipAnsiSequence(runes, 4)
		expected := 6
		if result != expected {
			t.Errorf("Expected %d, got %d", expected, result)
		}
	})
}

// Benchmark tests.
func BenchmarkGetDisplayWidth(b *testing.B) {
	testStrings := []string{
		"Simple text",
		"Text\twith\ttabs",
		"\033[31mColored text\033[0m",
		"Mixed content: tab\there, color\033[32mgreen\033[0m, unicode café",
	}

	for _, s := range testStrings {
		b.Run(s, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				getDisplayWidth(s)
			}
		})
	}
}

func BenchmarkContentFitsTerminal(b *testing.B) {
	// Save original sizer
	originalSizer := terminalSizer
	defer func() { terminalSizer = originalSizer }()
	terminalSizer = MockTerminalSizer{Width: 80, Height: 24}

	content := strings.Repeat("Line of text\n", 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ContentFitsTerminal(content)
	}
}

// Integration test.
func TestIntegration(t *testing.T) {
	// Save original sizer
	originalSizer := terminalSizer
	defer func() { terminalSizer = originalSizer }()

	t.Run("realistic terminal scenario", func(t *testing.T) {
		terminalSizer = MockTerminalSizer{Width: 80, Height: 24}

		// Test various content types
		testCases := []struct {
			name    string
			content string
			fits    bool
		}{
			{"short message", "Hello, World!", true},
			{"code snippet", "func main() {\n\tfmt.Println(\"Hello\")\n}", true},
			{"long line", strings.Repeat("x", 100), false},
			{"many lines", strings.Repeat("line\n", 30), false},
			{"ANSI colored output", "\033[32mSuccess:\033[0m Operation completed", true},
			{"mixed content", "Header\n\tIndented line\n\033[31mError:\033[0m Failed", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := ContentFitsTerminal(tc.content)
				if result != tc.fits {
					t.Errorf("ContentFitsTerminal(%q) = %v, expected %v", tc.name, result, tc.fits)
				}
			})
		}
	})
}
