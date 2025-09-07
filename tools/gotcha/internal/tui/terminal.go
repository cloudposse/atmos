package tui

import (
	"os"

	"golang.org/x/term"
)

// getTerminalWidth gets the current terminal width using golang.org/x/term.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a reasonable default if we can't detect
		return 80
	}
	return width
}

// getDisplayWidth calculates the actual display width of a string, ignoring ANSI escape sequences.
func getDisplayWidth(s string) int {
	width := 0
	i := 0
	runes := []rune(s)

	for i < len(runes) {
		r := runes[i]

		switch r {
		case '\033': // ESC character - start of ANSI escape sequence
			// Skip the entire ANSI escape sequence
			i = skipAnsiSequence(runes, i)
		default:
			// Count printable characters
			if r >= 32 && r < 127 {
				width++
			} else if r > 127 {
				// Unicode characters - count as 1 for simplicity
				// (proper width calculation would need wcwidth)
				width++
			}
			i++
		}
	}

	return width
}

// skipAnsiSequence skips over an ANSI escape sequence starting at the given position.
func skipAnsiSequence(runes []rune, start int) int {
	i := start
	if i >= len(runes) || runes[i] != '\033' {
		return i
	}

	i++ // Skip ESC
	if i >= len(runes) {
		return i
	}

	// Check for CSI (Control Sequence Introducer) - ESC[
	if runes[i] == '[' {
		return skipCSISequence(runes, i+1)
	}

	// Skip other escape sequences (OSC, etc.) - just skip to next letter
	for i < len(runes) && !isLetter(runes[i]) {
		i++
	}
	if i < len(runes) {
		i++ // Skip the final letter
	}

	return i
}

// skipCSISequence skips a CSI sequence (ESC[ ... letter).
func skipCSISequence(runes []rune, start int) int {
	i := start

	// Skip all parameter bytes (0-9, ;, etc.)
	for i < len(runes) && isCSIParameter(runes[i]) {
		i++
	}

	// Skip the final byte (letter)
	if i < len(runes) && isLetter(runes[i]) {
		i++
	}

	return i
}

// isCSIParameter checks if a rune is a valid CSI parameter character.
func isCSIParameter(r rune) bool {
	return (r >= '0' && r <= '9') || r == ';' || r == ':' || r == '?' || r == ' ' || r == '"' || r == '\''
}

// isLetter checks if a rune is a letter (simplified check).
func isLetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// contains checks if a string is in a slice.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
