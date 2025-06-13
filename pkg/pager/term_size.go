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

// getTerminalSize gets the current terminal dimensions.
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

// ContentFitsTerminal checks if the given string content fits within terminal dimensions.
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

// This handles tabs, ANSI escape sequences, and other special characters.
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

// skipAnsiSequence skips over ANSI escape sequences and returns the next index.
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

// skipCSISequence handles CSI (Control Sequence Introducer) sequences.
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

// isCSIParameter checks if a rune is a valid CSI parameter character.
func isCSIParameter(r rune) bool {
	return (r >= '0' && r <= '9') || r == ';' || r == ' ' || r == '?' || r == '!'
}
