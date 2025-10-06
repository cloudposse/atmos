package errors

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cockroachdb/errors"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultMaxLineLength is the default maximum line length before wrapping.
	DefaultMaxLineLength = 80

	// Newline is used for joining wrapped lines.
	newline = "\n"
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
	defer perf.Track(nil, "errors.DefaultFormatterConfig")()

	return FormatterConfig{
		Verbose:       false,
		Color:         "auto",
		MaxLineLength: DefaultMaxLineLength,
	}
}

// renderHintWithMarkdown renders a hint using the configured Atmos markdown renderer.
func renderHintWithMarkdown(hint string, renderer *markdown.Renderer) string {
	// Render hint text with emoji and markdown support.
	hintText := "💡 " + hint
	if renderer != nil {
		rendered, err := renderer.RenderWithoutWordWrap(hintText)
		if err == nil {
			// Add 4-space indent after rendering (consistent with Atmos list style).
			trimmed := strings.TrimRight(rendered, " \n\t")
			return "    " + trimmed
		}
	}
	// Fallback: 4-space indent with plain text.
	return "    " + hintText
}

// formatContextTable creates a styled 2-column table for error context.
// Context is extracted from cockroachdb/errors safe details and displayed
// as key-value pairs in verbose mode.
func formatContextTable(err error, useColor bool) string {
	details := errors.GetSafeDetails(err)
	if len(details.SafeDetails) == 0 {
		return ""
	}

	// Parse "component=vpc stack=prod" format into key-value pairs.
	var rows [][]string
	for _, detail := range details.SafeDetails {
		str := fmt.Sprintf("%v", detail)
		pairs := strings.Split(str, " ")
		for _, pair := range pairs {
			if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
				rows = append(rows, []string{parts[0], parts[1]})
			}
		}
	}

	if len(rows) == 0 {
		return ""
	}

	// Create styled table.
	t := table.New().
		Border(lipgloss.ThickBorder()).
		Headers("Context", "Value").
		Rows(rows...)

	if useColor {
		t = t.
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
			StyleFunc(func(row, col int) lipgloss.Style {
				style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
				if row == -1 {
					// Header row - green and bold.
					return style.Foreground(lipgloss.Color(theme.ColorGreen)).Bold(true)
				}
				if col == 0 {
					// Key column - dimmed gray.
					return style.Foreground(lipgloss.Color("#808080"))
				}
				// Value column - normal.
				return style
			})
	}

	return "\n" + t.String()
}

// Format formats an error for display with smart chain handling.
func Format(err error, config FormatterConfig) string {
	defer perf.Track(nil, "errors.Format")()

	if err == nil {
		return ""
	}

	// Determine if we should use color.
	useColor := shouldUseColor(config.Color)

	// Define styles.
	errorStyle := lipgloss.NewStyle()

	if useColor {
		errorStyle = errorStyle.Foreground(lipgloss.Color("#FF0000")) // Red
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
			if useColor {
				// Use the configured Atmos markdown renderer.
				rendered := renderHintWithMarkdown(hint, GetMarkdownRenderer())
				output.WriteString(rendered)
			} else {
				// Use 4-space indent for consistency.
				output.WriteString("    💡 " + hint)
			}
			output.WriteString("\n")
		}
	}

	// In verbose mode, show context table and full stack trace.
	if config.Verbose {
		contextTable := formatContextTable(err, useColor)
		if contextTable != "" {
			output.WriteString(contextTable)
			output.WriteString(newline)
		}
		output.WriteString(newline)
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
		// Check if stderr is a TTY.
		return term.IsTerminal(int(os.Stderr.Fd()))
	default:
		return term.IsTerminal(int(os.Stderr.Fd()))
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

	return strings.Join(lines, newline)
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
