package errors

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cockroachdb/errors"
	"golang.org/x/term"
)

const (
	// DefaultMaxLineLength is the default maximum line length before wrapping.
	DefaultMaxLineLength = 80
)

// FormatterConfig controls error formatting behavior.
type FormatterConfig struct {
	// Verbose enables detailed error chain output.
	Verbose bool

	// Color controls color output: "auto", "always", or "never".
	Color string

	// MaxLineLength is the maximum length before wrapping (default: 80).
	MaxLineLength int
}

// DefaultFormatterConfig returns default formatting configuration.
func DefaultFormatterConfig() FormatterConfig {
	return FormatterConfig{
		Verbose:       false,
		Color:         "auto",
		MaxLineLength: DefaultMaxLineLength,
	}
}

// Format formats an error for display with smart chain handling.
func Format(err error, config FormatterConfig) string {
	if err == nil {
		return ""
	}

	// Determine if we should use color.
	useColor := shouldUseColor(config.Color)

	// Define styles.
	var (
		errorStyle = lipgloss.NewStyle()
		hintStyle  = lipgloss.NewStyle()
	)

	if useColor {
		errorStyle = errorStyle.Foreground(lipgloss.Color("#FF0000")) // Red
		hintStyle = hintStyle.Foreground(lipgloss.Color("#00FFFF"))   // Cyan
	}

	var output strings.Builder

	// Get the main error message.
	mainMsg := err.Error()

	// Check if message is too long for single line.
	if len(mainMsg) > config.MaxLineLength && !config.Verbose {
		// Wrap long messages.
		wrapped := wrapText(mainMsg, config.MaxLineLength)
		output.WriteString(errorStyle.Render(wrapped))
	} else {
		output.WriteString(errorStyle.Render(mainMsg))
	}

	// Add hints if present.
	hints := errors.GetAllHints(err)
	if len(hints) > 0 {
		output.WriteString("\n")
		for _, hint := range hints {
			hintLine := "  💡 " + hint
			output.WriteString(hintStyle.Render(hintLine))
			output.WriteString("\n")
		}
	}

	// In verbose mode, show the full stack trace.
	if config.Verbose {
		output.WriteString("\n\n")
		output.WriteString(formatStackTrace(err, useColor))
	}

	return output.String()
}

// shouldUseColor determines if color output should be used.
func shouldUseColor(colorMode string) bool {
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	case "auto":
		// Check if stdout is a TTY.
		return term.IsTerminal(int(os.Stdout.Fd()))
	default:
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
}

// wrapText wraps text to the specified width.
func wrapText(text string, width int) string {
	if width <= 0 {
		width = DefaultMaxLineLength
	}

	var lines []string
	var currentLine strings.Builder

	words := strings.Fields(text)
	for i, word := range words {
		// Check if adding this word would exceed the width.
		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += " " + word
		} else {
			testLine = word
		}

		if len(testLine) > width && currentLine.Len() > 0 {
			// Start a new line.
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		} else {
			if i > 0 && currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
	}

	// Add the last line.
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}

// formatStackTrace formats the full error chain with stack traces.
func formatStackTrace(err error, useColor bool) string {
	style := lipgloss.NewStyle()
	if useColor {
		style = style.Foreground(lipgloss.Color("#808080")) // Gray
	}

	// Use cockroachdb/errors format with stack traces.
	details := fmt.Sprintf("%+v", err)
	return style.Render(details)
}
