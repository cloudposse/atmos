package utils

import (
	"bytes"
	"os"

	"github.com/alecthomas/chroma/quick"
	"github.com/arsham/figurine/figurine"
	"github.com/charmbracelet/glamour"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/jwalton/go-supportscolor"
)

// HighlightCode returns a syntax highlighted code for the specified language
func HighlightCode(code string, language string, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, code, language, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PrintStyledText prints a styled text to the terminal
func PrintStyledText(text string) error {
	// Check if the terminal supports colors
	if supportscolor.Stdout().SupportsColor {
		return figurine.Write(os.Stdout, text, "ANSI Regular.flf")
	}
	return nil
}

// RenderMarkdown renders markdown text with terminal styling
func RenderMarkdown(markdown string, style string) (string, error) {
	// If no style is provided, use the default style
	if style == "" {
		style = "dark"
	}

	termWriter := term.NewResponsiveWriter(os.Stdout)
	screenWidth := termWriter.(*term.TerminalWriter).GetWidth()

	// Create a new renderer with the specified style
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(int(screenWidth)),
	)
	if err != nil {
		return "", err
	}

	return r.Render(markdown)
}
