package markdown

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

// Cloud Posse color constants for code blocks.
const (
	CPLightGray      = "#e7e5e4" // Light gray for primary text
	CPMidGray        = "#57534e" // Mid gray for secondary text
	CPDarkGray       = "#030014" // Very dark gray (not used for backgrounds)
	CPCodeBackground = "#2F2E36" // Medium gray for code block backgrounds (from Fang) - RGB(47,46,54)
	CPCodeBgANSI256  = "236"     // ANSI256 color 236 = RGB(48,48,48) - closest to #2F2E36
	CPPurple         = "#9B51E0" // Purple accent (existing Atmos purple)
	CPWhite          = "#FFFFFF" // White for emphasis
)

// CodeblockStyle defines the styling for code blocks in help text.
type CodeblockStyle struct {
	Base    lipgloss.Style
	Text    lipgloss.Style
	Comment lipgloss.Style
	Program lipgloss.Style
	Flag    lipgloss.Style
	Arg     lipgloss.Style
}

// NewCodeblockStyle creates a new code block style with Cloud Posse colors.
// Uses the provided renderer to ensure correct color profile.
// Following Fang's approach: ALL styles have the SAME background to avoid nested backgrounds.
func NewCodeblockStyle(renderer *lipgloss.Renderer) CodeblockStyle {
	// Bind environment variable for debug logging.
	_ = viper.BindEnv("ATMOS_DEBUG_COLORS")

	// Debug: Check renderer profile.
	debugColors := viper.GetString("ATMOS_DEBUG_COLORS") != ""
	if debugColors {
		fmt.Fprintf(os.Stderr, "[DEBUG] NewCodeblockStyle - Renderer Profile: %v\n", renderer.ColorProfile())
	}

	// Use AdaptiveColor with hex values. Setting both Light and Dark to the same ensures consistency.
	bgColor := lipgloss.AdaptiveColor{Light: CPCodeBackground, Dark: CPCodeBackground}
	lightGray := lipgloss.AdaptiveColor{Light: CPLightGray, Dark: CPLightGray}
	midGray := lipgloss.AdaptiveColor{Light: CPMidGray, Dark: CPMidGray}
	purple := lipgloss.AdaptiveColor{Light: CPPurple, Dark: CPPurple}

	return CodeblockStyle{
		Base: renderer.NewStyle().
			Background(bgColor).
			Foreground(lightGray).
			Padding(1, 2).
			MarginLeft(2), // Add 2 spaces left margin
		// IMPORTANT: All inner styles MUST have the same background as Base
		// This prevents nested backgrounds when rendering
		Text: renderer.NewStyle().
			Background(bgColor).
			Foreground(lightGray),
		Comment: renderer.NewStyle().
			Background(bgColor).
			Foreground(midGray),
		Program: renderer.NewStyle().
			Background(bgColor).
			Foreground(purple).
			Bold(true),
		Flag: renderer.NewStyle().
			Background(bgColor).
			Foreground(purple),
		Arg: renderer.NewStyle().
			Background(bgColor).
			Foreground(lightGray).
			Italic(true),
	}
}

// RenderCodeBlock renders a code block with Fang-style syntax highlighting.
// This avoids nested backgrounds by doing custom parsing instead of using Glamour.
// It follows Fang's approach: style content first, measure it, then apply background.
func RenderCodeBlock(renderer *lipgloss.Renderer, content string, terminalWidth int) string {
	style := NewCodeblockStyle(renderer)

	// Split content into lines and style each one
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var styledLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			styledLines = append(styledLines, "")
			continue
		}

		// Style the line based on its content (returns ANSI-styled text)
		styledLine := styleLine(line, &style)
		styledLines = append(styledLines, styledLine)
	}

	// Join styled lines
	styledContent := strings.Join(styledLines, "\n")

	// Calculate block width based on content (Fang's approach)
	padding := style.Base.GetHorizontalPadding()
	contentWidth := lipgloss.Width(styledContent) // Measures visible width, excludes ANSI
	blockWidth := contentWidth + padding

	// Cap at terminal width
	if blockWidth > terminalWidth {
		blockWidth = terminalWidth
	}

	// Apply the block style with background and padding
	// Since all inner styles have the same background, there's no nesting
	blockStyle := style.Base.Width(blockWidth)
	return blockStyle.Render(styledContent)
}

// styleLine applies syntax highlighting to a single line.
func styleLine(line string, style *CodeblockStyle) string {
	trimmed := strings.TrimSpace(line)

	// Comment lines
	if strings.HasPrefix(trimmed, "#") {
		return style.Comment.Render(line)
	}

	// Command lines with $ prefix
	if strings.HasPrefix(trimmed, "$") {
		return styleCommandLine(line, style)
	}

	// Regular text
	return style.Text.Render(line)
}

// styleCommandLine styles a command line with syntax highlighting for program, flags, and args.
func styleCommandLine(line string, style *CodeblockStyle) string {
	// Preserve leading whitespace
	leadingSpace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	trimmed := stripDollarPrefix(strings.TrimSpace(line))

	// Split into tokens
	tokens := tokenize(trimmed)
	styledTokens := styleTokens(tokens, style)

	// Join tokens with styled spaces (spaces must have the same background as tokens)
	styledSpace := style.Text.Render(" ")
	result := style.Comment.Render("$ ") + strings.Join(styledTokens, styledSpace)
	return leadingSpace + result
}

// stripDollarPrefix removes $ or "$ " prefix from a line.
func stripDollarPrefix(line string) string {
	if strings.HasPrefix(line, "$ ") {
		return line[2:]
	}
	if strings.HasPrefix(line, "$") {
		return line[1:]
	}
	return line
}

// styleTokens applies appropriate styling to each token based on its type.
func styleTokens(tokens []string, style *CodeblockStyle) []string {
	var styledTokens []string

	for i, token := range tokens {
		if token == "" {
			continue
		}

		styledToken := styleToken(token, i, style)
		styledTokens = append(styledTokens, styledToken)
	}

	return styledTokens
}

// styleToken applies styling to a single token based on its position and content.
func styleToken(token string, position int, style *CodeblockStyle) string {
	// First token is the program name
	if position == 0 {
		return style.Program.Render(token)
	}

	// Flags start with - or --
	if strings.HasPrefix(token, "-") {
		return style.Flag.Render(token)
	}

	// Arguments in angle brackets or square brackets
	if (strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">")) ||
		(strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]")) {
		return style.Arg.Render(token)
	}

	// Regular text
	return style.Text.Render(token)
}

// tokenize splits a command line into tokens, respecting quotes.
func tokenize(line string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range line {
		if isQuoteStart(ch, inQuote) {
			inQuote, quoteChar = true, ch
			current.WriteRune(ch)
		} else if ch == quoteChar && inQuote {
			inQuote, quoteChar = false, 0
			current.WriteRune(ch)
		} else if ch == ' ' && !inQuote {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(ch)
		}
	}

	// Add final token if exists
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// isQuoteStart checks if a character starts a quote.
func isQuoteStart(ch rune, inQuote bool) bool {
	return (ch == '"' || ch == '\'') && !inQuote
}
