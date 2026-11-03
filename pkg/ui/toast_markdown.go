package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
)

// toastMarkdown renders markdown text with preserved newlines, an icon prefix, and auto-indents multi-line content.
// Uses a compact stylesheet for toast-style inline formatting.
func (f *formatter) toastMarkdown(icon string, style *lipgloss.Style, text string) (string, error) {
	// Render markdown with toast-specific compact stylesheet.
	rendered, err := f.renderToastMarkdown(text)
	if err != nil {
		return "", err
	}

	// Style the icon if color is supported.
	styledIcon := icon
	if f.SupportsColor() && style != nil {
		styledIcon = style.Render(icon)
	}

	lines := toastMarkdownLines(rendered)
	if len(lines) == 0 {
		// Empty message: return icon + space for consistency with iconMessageFormat.
		return fmt.Sprintf(iconMessageFormat, styledIcon, ""), nil
	}

	// Multi-line: first line with icon, rest indented to align under first line's text.
	result := fmt.Sprintf(iconMessageFormat, styledIcon, lines[0])
	// Calculate indent: icon width + 1 space from iconMessageFormat.
	// Use lipgloss.Width to handle multi-cell characters like emojis.
	indent := strings.Repeat(space, lipgloss.Width(icon)+1)
	for _, line := range lines[1:] {
		result += newline + indent + line
	}
	return result, nil
}

// toastMarkdownLines removes document margins while retaining Markdown indentation.
func toastMarkdownLines(rendered string) []string {
	// Split by newlines and trim trailing padding that Glamour adds.
	lines := atmosansi.TrimTrailingWhitespace(rendered, paragraphIndent, paragraphIndentWidth)
	blank := func(line string) bool { return strings.TrimSpace(ansi.Strip(line)) == "" }
	// Glamour's document and list blocks add different numbers of blank margin lines.
	// Strip both ends after rendering instead of assuming one leading newline.
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	// Intentional trailing newlines are also stripped because they would cause
	// extra indented empty lines in multi-line output.
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	// Remove only the common document indent. Trimming every line's leading
	// spaces would flatten nested lists and destroy wrapped-item alignment.
	commonIndent := toastMarkdownIndent(lines)
	for i, line := range lines {
		if blank(line) {
			lines[i] = ""
		} else if commonIndent > 0 {
			// ANSI-aware truncation retains styling that spans the removed spaces.
			lines[i] = ansi.TruncateLeft(line, commonIndent, "")
		}
	}
	return lines
}

// toastMarkdownIndent finds the common leading space count of nonempty lines.
func toastMarkdownIndent(lines []string) int {
	commonIndent := len(lines[0])
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		indent := len(plain) - len(strings.TrimLeft(plain, space))
		commonIndent = min(commonIndent, indent)
	}
	return commonIndent
}
