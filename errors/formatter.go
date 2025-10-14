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
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultMaxLineLength is the default maximum line length before wrapping.
	DefaultMaxLineLength = 80

	// DefaultMarkdownWidth is the default width for markdown rendering when config is not available.
	DefaultMarkdownWidth = 120

	// Space is used for separating words.
	space = " "

	// Newline is used for line breaks.
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

// Format formats an error for display with structured markdown sections.
func Format(err error, config FormatterConfig) string {
	defer perf.Track(nil, "errors.Format")()

	if err == nil {
		return ""
	}

	// Build structured markdown document with sections.
	md := buildMarkdownSections(err, config)

	// Render markdown through Glamour.
	return renderMarkdown(md)
}

// buildMarkdownSections builds the complete markdown document with all sections.
func buildMarkdownSections(err error, config FormatterConfig) string {
	var md strings.Builder

	// Section 1: Error header + message.
	// Extract custom title or use default.
	title := extractCustomTitle(err)
	if title == "" {
		title = "Error"
	}
	md.WriteString("# " + title + newline + newline)
	md.WriteString("**Error:** " + err.Error() + newline + newline)

	// Section 2: Explanation.
	addExplanationSection(&md, err)

	// Section 3 & 4: Examples and Hints.
	addExampleAndHintsSection(&md, err)

	// Section 5: Context.
	addContextSection(&md, err)

	// Section 6: Stack trace (verbose mode only).
	if config.Verbose {
		addStackTraceSection(&md, err)
	}

	return md.String()
}

// addExplanationSection adds the explanation section if details exist.
func addExplanationSection(md *strings.Builder, err error) {
	details := errors.GetAllDetails(err)
	if len(details) > 0 {
		md.WriteString(newline + newline + "## Explanation" + newline + newline)
		for _, detail := range details {
			fmt.Fprintf(md, "%s"+newline, detail)
		}
		md.WriteString(newline)
	}
}

// extractCustomTitle extracts the custom title from error hints.
func extractCustomTitle(err error) string {
	allHints := errors.GetAllHints(err)
	for _, hint := range allHints {
		if strings.HasPrefix(hint, "TITLE:") {
			return strings.TrimPrefix(hint, "TITLE:")
		}
	}
	return ""
}

// addExampleAndHintsSection separates hints into examples and regular hints, then adds both sections.
func addExampleAndHintsSection(md *strings.Builder, err error) {
	allHints := errors.GetAllHints(err)
	var examples []string
	var hints []string

	// Separate hints into examples, title, and regular hints.
	for _, hint := range allHints {
		switch {
		case strings.HasPrefix(hint, "TITLE:"):
			// Skip title hints - they're extracted separately.
			continue
		case strings.HasPrefix(hint, "EXAMPLE:"):
			examples = append(examples, strings.TrimPrefix(hint, "EXAMPLE:"))
		default:
			hints = append(hints, hint)
		}
	}

	// Add Example section.
	if len(examples) > 0 {
		md.WriteString(newline + newline + "## Example" + newline + newline)
		for _, example := range examples {
			md.WriteString(example)
			if !strings.HasSuffix(example, newline) {
				md.WriteString(newline)
			}
		}
		md.WriteString(newline)
	}

	// Add Hints section.
	// IMPORTANT: Each hint MUST be on its own line with a blank line after it to prevent
	// markdown renderers from collapsing multiple hints into a single paragraph.
	// We NEVER delete newlines - only trailing spaces/tabs before newlines are removed.
	//
	// Line breaks and spacing should be controlled by:
	//   - Markdown content itself (blank lines between paragraphs, etc.)
	//   - Markdown stylesheets (renderer configuration)
	//   - NOT by post-processing that removes newlines
	if len(hints) > 0 {
		md.WriteString(newline + newline + "## Hints" + newline + newline)
		for _, hint := range hints {
			// Add blank line after each hint to ensure proper line breaks in markdown rendering.
			md.WriteString("💡 " + hint + newline + newline)
		}
	}
}

// addContextSection adds the context section if context exists.
func addContextSection(md *strings.Builder, err error) {
	context := formatContextForMarkdown(err)
	if context != "" {
		md.WriteString(newline + newline + "## Context" + newline + newline)
		md.WriteString(context)
		md.WriteString(newline)
	}
}

// addStackTraceSection adds the stack trace section in verbose mode.
func addStackTraceSection(md *strings.Builder, err error) {
	md.WriteString(newline + newline + "## Stack Trace" + newline + newline)
	md.WriteString("```" + newline)
	fmt.Fprintf(md, "%+v", err)
	md.WriteString(newline + "```" + newline)
}

// renderMarkdown renders markdown string through Glamour or creates a minimal renderer.
func renderMarkdown(md string) string {
	renderer := GetMarkdownRenderer()
	if renderer == nil {
		// Create minimal renderer with default config when global renderer not initialized.
		// This happens during early errors before atmos config is loaded.
		defaultConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Docs: schema.Docs{
					MaxWidth: DefaultMarkdownWidth,
				},
			},
		}
		var err error
		renderer, err = markdown.NewTerminalMarkdownRenderer(defaultConfig)
		if err != nil {
			// Last resort fallback: return plain markdown.
			return md
		}
	}

	rendered, renderErr := renderer.RenderErrorf(md)
	if renderErr == nil {
		return rendered
	}

	// Fallback to plain markdown.
	return md
}

// formatContextForMarkdown formats context as a markdown table.
func formatContextForMarkdown(err error) string {
	details := errors.GetSafeDetails(err)
	if len(details.SafeDetails) == 0 {
		return ""
	}

	// Parse "component=vpc stack=prod" format into key-value pairs.
	var rows []string
	for _, detail := range details.SafeDetails {
		str := fmt.Sprintf("%v", detail)
		pairs := strings.Split(str, " ")
		for _, pair := range pairs {
			if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
				rows = append(rows, fmt.Sprintf("| %s | %s |", parts[0], parts[1]))
			}
		}
	}

	// Return empty if no rows were parsed.
	if len(rows) == 0 {
		return ""
	}

	// Build table with header.
	var md strings.Builder
	md.WriteString("| Key | Value |\n")
	md.WriteString("|-----|-------|\n")
	for _, row := range rows {
		md.WriteString(row + "\n")
	}

	return md.String()
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

// wrapText wraps text to the specified width while preserving intentional line breaks.
// IMPORTANT: This function should NOT be used on text with intentional newlines,
// as strings.Fields() splits on ALL whitespace including newlines, which destroys
// the original line break structure. Only use this for wrapping single paragraphs.
// NEVER call this on multi-line text that needs to preserve its line break structure.
//
// Line breaks and spacing should be controlled by:
//   - Markdown content itself (blank lines between paragraphs, etc.)
//   - Markdown stylesheets (renderer configuration)
//   - NOT by post-processing that removes newlines
func wrapText(text string, width int) string {
	if width <= 0 {
		width = DefaultMaxLineLength
	}

	var lines []string
	var currentLine strings.Builder

	// WARNING: strings.Fields() removes ALL whitespace including newlines.
	// This destroys intentional line breaks in the input text.
	// Only use this function on single-paragraph text.
	words := strings.Fields(text)
	for i, word := range words {
		// Check if adding this word would exceed the width.
		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += space + word
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
				currentLine.WriteString(space)
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
