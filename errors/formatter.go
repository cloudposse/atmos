package errors

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
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

	// MaxLineLength is the maximum length before wrapping (default: 80).
	// This controls both the width passed to the markdown renderer and the
	// wrapping of text in explanation and hint sections.
	MaxLineLength int

	// Title is an optional custom title for the error message.
	Title string
}

// DefaultFormatterConfig returns default formatting configuration.
func DefaultFormatterConfig() FormatterConfig {
	return FormatterConfig{
		Verbose:       false,
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

	// Create styled table with width constraint.
	t := table.New().
		Border(lipgloss.ThickBorder()).
		Headers("Context", "Value").
		Rows(rows...).
		Width(DefaultMarkdownWidth)

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

	// Determine color usage from terminal settings.
	// This respects --no-color, --force-color, NO_COLOR env var, and terminal.color config.
	useColor := shouldUseColor()

	// Build structured markdown document with sections.
	md := buildMarkdownSections(err, config, useColor)

	// Render markdown through Glamour with configured width.
	rendered := renderMarkdown(md, config.MaxLineLength)

	// Strip ANSI codes if color is disabled.
	if !useColor {
		rendered = stripANSI(rendered)
	}

	return rendered
}

// buildMarkdownSections builds the complete markdown document with all sections.
func buildMarkdownSections(err error, config FormatterConfig, useColor bool) string {
	var md strings.Builder

	// Section 1: Error header + message.
	// Prefer config.Title, then extract custom title, or use default.
	title := config.Title
	if title == "" {
		title = extractCustomTitle(err)
	}
	if title == "" {
		title = "Error"
	}
	md.WriteString("# " + title + newline + newline)

	// Extract sentinel error and wrapped message.
	sentinelMsg, wrappedMsg := extractSentinelAndWrappedMessage(err)

	// Check for specific error types that need special formatting.
	// Priority order: WorkflowStepError > ExecError > generic errors.
	var workflowErr *WorkflowStepError
	var execErr *ExecError

	switch {
	case errors.As(err, &workflowErr):
		// Workflow orchestration failures - show workflow-specific message with exit code.
		md.WriteString(fmt.Sprintf("**Error:** %s%s%s", workflowErr.WorkflowStepMessage(), newline, newline))
	case errors.As(err, &execErr):
		// External command execution failures - show command and exit code.
		md.WriteString(fmt.Sprintf("**Error:** %s with exit code %d%s%s", sentinelMsg, execErr.ExitCode, newline, newline))
	default:
		// All other errors - just show the sentinel message without exit code.
		md.WriteString("**Error:** " + sentinelMsg + newline + newline)
	}

	// Section 2: Explanation.
	addExplanationSection(&md, err, wrappedMsg, config.MaxLineLength)

	// Section 3 & 4: Examples and Hints.
	addExampleAndHintsSection(&md, err, config.MaxLineLength)

	// Section 4.5: Command Output (for ExecError with stderr).
	addCommandOutputSection(&md, err)

	// Section 5: Context.
	addContextSection(&md, err, useColor)

	// Section 6: Stack trace (verbose mode only).
	if config.Verbose {
		addStackTraceSection(&md, err, useColor)
	}

	return md.String()
}

// extractSentinelAndWrappedMessage extracts the root sentinel error message
// and any wrapped context message from the error chain.
// For example, given: fmt.Errorf("%w: The command has no steps", ErrInvalidArguments)
// Returns: ("invalid arguments", "The command has no steps").
func extractSentinelAndWrappedMessage(err error) (sentinelMsg string, wrappedMsg string) {
	if err == nil {
		return "", ""
	}

	// Get the full error message.
	fullMsg := err.Error()

	// Unwrap to find the root sentinel error.
	current := err
	for {
		unwrapped := errors.Unwrap(current)
		if unwrapped == nil {
			// Reached the root error (sentinel).
			sentinelMsg = current.Error()
			break
		}
		current = unwrapped
	}

	// Extract the wrapped message by removing the sentinel prefix.
	// The format from fmt.Errorf("%w: message", sentinel) is "sentinel: message".
	if strings.HasPrefix(fullMsg, sentinelMsg+": ") {
		wrappedMsg = strings.TrimPrefix(fullMsg, sentinelMsg+": ")
	} else {
		// If no wrapped message, just use sentinel.
		sentinelMsg = fullMsg
	}

	return sentinelMsg, wrappedMsg
}

// addExplanationSection adds the explanation section if details or wrapped message exist.
func addExplanationSection(md *strings.Builder, err error, wrappedMsg string, maxLineLength int) {
	// maxLineLength is unused here because the markdown renderer handles wrapping.
	_ = maxLineLength

	details := errors.GetAllDetails(err)
	hasContent := len(details) > 0 || wrappedMsg != ""

	if hasContent {
		md.WriteString(newline + newline + "## Explanation" + newline + newline)

		// Add wrapped message first if present.
		// Don't wrap - let the markdown renderer handle it to preserve structure.
		if wrappedMsg != "" {
			md.WriteString(wrappedMsg + newline + newline)
		}

		// Add details from error chain.
		// Don't wrap - let the markdown renderer handle it to preserve code blocks and newlines.
		for _, detail := range details {
			md.WriteString(fmt.Sprintf("%v", detail) + newline)
		}

		if len(details) > 0 {
			md.WriteString(newline)
		}
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

// categorizeHints separates hints into examples and regular hints, filtering out empty hints.
func categorizeHints(allHints []string) (examples []string, hints []string) {
	for _, hint := range allHints {
		switch {
		case strings.HasPrefix(hint, "TITLE:"):
			// Skip title hints - they're extracted separately.
			continue
		case strings.HasPrefix(hint, "EXAMPLE:"):
			examples = append(examples, strings.TrimPrefix(hint, "EXAMPLE:"))
		default:
			// Skip empty or whitespace-only hints.
			if trimmed := strings.TrimSpace(hint); trimmed != "" {
				hints = append(hints, hint)
			}
		}
	}
	return examples, hints
}

// addExampleAndHintsSection separates hints into examples and regular hints, then adds both sections.
func addExampleAndHintsSection(md *strings.Builder, err error, maxLineLength int) {
	allHints := errors.GetAllHints(err)
	examples, hints := categorizeHints(allHints)

	// Add Example section.
	if len(examples) > 0 {
		md.WriteString(newline + newline + "## Example" + newline + newline)
		for _, example := range examples {
			// Wrap examples in code fences to prevent markdown interpretation,
			// but only if they don't already have fences (for backward compatibility
			// with WithExampleFile which may include pre-fenced markdown content).
			hasFences := strings.HasPrefix(strings.TrimSpace(example), "```")
			if !hasFences {
				md.WriteString("```yaml" + newline)
			}
			md.WriteString(example)
			if !strings.HasSuffix(example, newline) {
				md.WriteString(newline)
			}
			if !hasFences {
				md.WriteString("```" + newline)
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
			// Don't wrap - let the markdown renderer handle it to preserve structure.
			// The maxLineLength parameter is unused here.
			_ = maxLineLength
			md.WriteString("💡 " + hint + newline + newline)
		}
	}
}

// addCommandOutputSection adds the command output section for ExecError with stderr.
func addCommandOutputSection(md *strings.Builder, err error) {
	var execErr *ExecError
	if errors.As(err, &execErr) && execErr.Stderr != "" {
		md.WriteString(newline + newline + "## Command Output" + newline + newline)
		md.WriteString("```" + newline)
		md.WriteString(execErr.Stderr)
		if !strings.HasSuffix(execErr.Stderr, newline) {
			md.WriteString(newline)
		}
		md.WriteString("```" + newline)
	}
}

// addContextSection adds the context section if context exists.
func addContextSection(md *strings.Builder, err error, useColor bool) {
	// Context is rendered as a markdown table, so we use formatContextForMarkdown.
	// The useColor parameter is available for future use if we need color-aware context rendering.
	_ = useColor
	context := formatContextForMarkdown(err)
	if context != "" {
		md.WriteString(newline + newline + "## Context" + newline + newline)
		md.WriteString(context)
		md.WriteString(newline)
	}
}

// addStackTraceSection adds the stack trace section in verbose mode.
func addStackTraceSection(md *strings.Builder, err error, useColor bool) {
	// Stack traces are rendered in code blocks, so color doesn't apply.
	// The useColor parameter is available for future use if needed.
	_ = useColor
	md.WriteString(newline + newline + "## Stack Trace" + newline + newline)
	md.WriteString("```" + newline)
	fmt.Fprintf(md, "%+v", err)
	md.WriteString(newline + "```" + newline)
}

// renderMarkdown renders markdown string through Glamour with specified width.
//
// This function creates a fresh markdown renderer for each call rather than using
// the global renderer from pkg/ui to ensure:
// 1. The FormatterConfig.MaxLineLength parameter is respected (global renderer may have different width)
// 2. Error formatting works before the UI system is initialized (early startup errors)
// 3. No circular dependencies (pkg/ui imports errors package)
func renderMarkdown(md string, maxLineLength int) string {
	// Use provided maxLineLength, or fall back to default if not set.
	width := maxLineLength
	if width <= 0 {
		width = DefaultMarkdownWidth
	}

	// Always create a fresh renderer with the specified width to ensure
	// MaxLineLength parameter is respected. The global renderer may have
	// a different width configured.
	config := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Docs: schema.Docs{
				MaxWidth: width,
			},
		},
	}

	renderer, err := markdown.NewTerminalMarkdownRenderer(config)
	if err != nil {
		// Fallback: return plain markdown if renderer creation fails.
		return md
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
// This uses the terminal package's color logic which respects:
// - --no-color, --color, --force-color flags
// - NO_COLOR, CLICOLOR, CLICOLOR_FORCE environment variables
// - settings.terminal.color and settings.terminal.no_color in atmos.yaml
func shouldUseColor() bool {
	// Build terminal config from all sources (flags, env vars, atmos.yaml).
	termConfig := &terminal.Config{
		NoColor:    viper.GetBool("no-color"),
		Color:      viper.GetBool("color"),
		ForceColor: viper.GetBool("force-color"),

		EnvNoColor:       os.Getenv("NO_COLOR") != "",
		EnvCLIColor:      os.Getenv("CLICOLOR"),
		EnvCLIColorForce: os.Getenv("CLICOLOR_FORCE") != "" || os.Getenv("FORCE_COLOR") != "",
	}

	// Add atmos.yaml settings if available.
	if atmosConfig != nil {
		termConfig.AtmosConfig = *atmosConfig
	}

	// Check if stderr is a TTY.
	isTTY := term.IsTerminal(int(os.Stderr.Fd()))

	// Use terminal package's color logic.
	return termConfig.ShouldUseColor(isTTY)
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	// ANSI escape code pattern: ESC [ ... m where ... can be numbers separated by semicolons.
	// More comprehensive pattern to catch all ANSI codes including SGR (colors/formatting).
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiPattern.ReplaceAllString(s, "")
}

// wrapText wraps text to the specified width while preserving intentional line breaks.
//
// DEPRECATED: This function is no longer used in production code.
// The markdown renderer (Glamour) handles text wrapping natively and correctly
// preserves markdown structure (code blocks, newlines, etc.). This function
// destroys markdown structure by calling strings.Fields() which removes ALL
// newlines, making it unsuitable for formatting error messages with code blocks.
//
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
