package pager

import (
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	tabWidth          = 8 // Standard tab width
	printableASCIIMin = 32
	printableASCIIMax = 127
)

// TermSize represents terminal dimensions.
type TermSize struct {
	Width  int
	Height int
}

// Interface for testability.
type TerminalSizer interface {
	GetSize(fd int) (width, height int, err error)
}

// Real implementation.
type RealTerminalSizer struct{}

func (r RealTerminalSizer) GetSize(fd int) (width, height int, err error) {
	return term.GetSize(fd)
}

// Mock implementation for testing.
type MockTerminalSizer struct {
	Width  int
	Height int
	Error  error
}

func (m MockTerminalSizer) GetSize(fd int) (width, height int, err error) {
	if m.Error != nil {
		return 0, 0, m.Error
	}
	return m.Width, m.Height, nil
}

// Global variable for dependency injection in tests.
var terminalSizer TerminalSizer = RealTerminalSizer{}

// getTerminalSize returns the current terminal's width and height.
// It retrieves the terminal size using the configured TerminalSizer and returns an error if the size cannot be determined.
func getTerminalSize() (TermSize, error) {
	w, h, errno := terminalSizer.GetSize(int(os.Stdout.Fd()))
	if errno != nil {
		return TermSize{}, errno
	}
	return TermSize{
		Width:  w,
		Height: h,
	}, nil
}

// ContentFitsTerminal returns true if the provided content fits within the current terminal's width and height, accounting for special characters and ANSI escape sequences.
func ContentFitsTerminal(content string) bool {
	termSize, err := getTerminalSize()
	if err != nil {
		// If we can't get terminal size, assume it doesn't fit
		return false
	}

	// Split content into lines
	lines := strings.Split(content, "\n")

	// Check height: number of lines should not exceed terminal height
	if len(lines) > termSize.Height {
		return false
	}

	// Check width: find the longest line and compare with terminal width
	maxWidth := 0
	for _, line := range lines {
		// Handle potential tab characters and other special characters
		lineWidth := getDisplayWidth(line)
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}
	// Content fits if max line width doesn't exceed terminal width
	return maxWidth <= termSize.Width
}

// getDisplayWidth returns the display width of a string, accounting for tabs, carriage returns, and ANSI escape sequences.
// Tabs are expanded to the next tab stop, carriage returns reset the width to zero, and ANSI escape sequences are ignored.
// Printable ASCII and most Unicode characters are counted as width 1, while control characters do not contribute to the width.
func getDisplayWidth(s string) int {
	width := 0
	i := 0
	runes := []rune(s)

	for i < len(runes) {
		r := runes[i]

		switch r {
		case '\t':
			// Tab aligns to next multiple of tabWidth
			width = ((width / tabWidth) + 1) * tabWidth
			i++
		case '\r':
			// Carriage return resets to beginning of line
			width = 0
			i++
		case '\033': // ESC character - start of ANSI escape sequence
			// Skip the entire ANSI escape sequence
			i = skipAnsiSequence(runes, i)
		default:
			// Count printable characters
			if r >= printableASCIIMin && r < printableASCIIMax {
				width++
			} else if r > printableASCIIMax {
				// Basic handling for Unicode - most characters are width 1
				// For more accurate width calculation, you might want to use a library
				// like github.com/mattn/go-runewidth
				width++
			}
			// Control characters (0-31) don't add to width
			i++
		}
	}

	return width
}

// skipAnsiSequence returns the index immediately after an ANSI escape sequence starting at the given position in the rune slice.
func skipAnsiSequence(runes []rune, start int) int {
	if start >= len(runes) || runes[start] != '\033' {
		return start + 1
	}

	i := start + 1
	if i >= len(runes) {
		return i
	}

	switch runes[i] {
	case '[':
		return skipCSISequence(runes, i)
	case '(', ')':
		return i + 2
	default:
		return i + 1
	}
}

// skipCSISequence skips over a CSI (Control Sequence Introducer) ANSI escape sequence in a slice of runes, starting at the '[' character, and returns the index immediately after the sequence.
func skipCSISequence(runes []rune, start int) int {
	i := start + 1 // skip '['

	// Skip parameters until we find the final character
	for i < len(runes) && isCSIParameter(runes[i]) {
		i++
	}

	// Skip the final character if present
	if i < len(runes) {
		i++
	}

	return i
}

// isCSIParameter returns true if the rune is a valid parameter character in an ANSI CSI sequence.
func isCSIParameter(r rune) bool {
	return (r >= '0' && r <= '9') || r == ';' || r == ' ' || r == '?' || r == '!'
}
