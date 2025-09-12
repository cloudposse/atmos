package tui

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// getTerminalWidth gets the current terminal width using golang.org/x/term.
func getTerminalWidth() int {
	// First try to get actual terminal size
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && width > 0 {
		return width
	}

	// Fallback to COLUMNS environment variable if set
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return w
		}
	}

	// Final fallback to a reasonable default
	return 80
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
