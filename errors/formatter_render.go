package errors

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// renderMarkdown renders markdown string through Glamour with specified width.
//
// This function creates a fresh markdown renderer for each call rather than using
// the global renderer from pkg/ui to ensure:
// 1. The FormatterConfig.MaxLineLength parameter is respected (global renderer may have different width)
// 2. Error formatting works before the UI system is initialized (early startup errors)
// 3. No circular dependencies (pkg/ui imports errors package).
func renderMarkdown(md string, maxLineLength int) (string, bool) {
	return renderWith(md, maxLineLength, (*markdown.Renderer).RenderErrorf)
}

// renderReadableMarkdown renders Markdown as plain text for output without styling.
// It avoids the ASCII preset used for non-TTY stderr, which leaves bold markers
// dangling around inline code and drops code-span backticks.
func renderReadableMarkdown(md string, maxLineLength int) (string, bool) {
	return renderWith(md, maxLineLength, func(r *markdown.Renderer, content string, _ ...interface{}) (string, error) {
		return r.RenderReadable(content)
	})
}

func renderWith(md string, maxLineLength int, render func(*markdown.Renderer, string, ...interface{}) (string, error)) (string, bool) {
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
			Terminal: schema.Terminal{
				MaxWidth: width,
			},
		},
	}

	renderer, err := newMarkdownRendererWithWidth(config, width)
	if err != nil {
		return "", false
	}

	rendered, renderErr := render(renderer, md)
	if renderErr == nil {
		return rendered, true
	}

	return "", false
}

// formatContextForMarkdown formats context as a markdown table.
// Uses GetAllSafeDetails to traverse the entire error chain.
func formatContextForMarkdown(err error) string {
	// Get all safe details from entire error chain, not just top-level.
	allDetails := errors.GetAllSafeDetails(err)
	var allSafeDetails []string
	for _, payload := range allDetails {
		allSafeDetails = append(allSafeDetails, payload.SafeDetails...)
	}
	if len(allSafeDetails) == 0 {
		return ""
	}

	// Parse "component=vpc stack=prod" format into key-value pairs.
	var rows []string
	for _, detail := range allSafeDetails {
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
// - settings.terminal.color and settings.terminal.no_color in atmos.yaml.
func shouldUseColor() bool {
	// Bind standard color environment signals locally, including during early startup.
	env := viper.New()
	for _, key := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR"} {
		_ = env.BindEnv(key, key)
	}

	// Build terminal config from all sources (flags, env vars, atmos.yaml).
	termConfig := &terminal.Config{
		NoColor:    terminalenv.ResolveNoColor(os.Args[1:], viper.GetBool("no-color")),
		Color:      viper.GetBool("color"),
		ForceColor: viper.GetBool("force-color"),

		EnvNoColor:       env.GetString("NO_COLOR") != "",
		EnvCLIColor:      env.GetString("CLICOLOR"),
		EnvCLIColorForce: env.GetString("CLICOLOR_FORCE") != "" || env.GetString("FORCE_COLOR") != "",
		//nolint:forbidigo // CI is the standard environment signal used by terminal color detection.
		EnvCI: os.Getenv("CI") != "",
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
// Deprecated: This function is no longer used in production code.
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
