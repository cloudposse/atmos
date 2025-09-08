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
