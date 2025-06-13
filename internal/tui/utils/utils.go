package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/chroma/quick"
	"github.com/arsham/figurine/figurine"
	"github.com/charmbracelet/glamour"
	"github.com/cloudposse/atmos/pkg/schema"
	mdstyle "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/jwalton/go-supportscolor"
	xterm "golang.org/x/term"
)

// HighlightCode returns a syntax highlighted code for the specified language
func HighlightCode(code string, language string, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, code, language, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PrintStyledText prints styled text to the terminal using the "ANSI Regular.flf" font if color output is supported.
func PrintStyledText(text string) error {
	// Check if the terminal supports colors
	if supportscolor.Stdout().SupportsColor {
		return figurine.Write(os.Stdout, text, "ANSI Regular.flf")
	}
	return nil
}

// PrintStyledTextToSpecifiedOutput writes styled text to the specified output writer using the "ANSI Regular.flf" font.
// Returns an error if writing fails.
func PrintStyledTextToSpecifiedOutput(out io.Writer, text string) error {
	return figurine.Write(out, text, "ANSI Regular.flf")
}

// RenderMarkdown converts a markdown string into styled terminal output using a custom style and word wrapping based on terminal width.
// Returns the rendered terminal string or an error if rendering fails.
func RenderMarkdown(markdownText string, style string) (string, error) {
	if markdownText == "" {
		return "", fmt.Errorf("empty markdown input")
	}

	// Get the custom style from atmos config
	customStyle, err := mdstyle.GetDefaultStyle(schema.AtmosConfiguration{})
	if err != nil {
		return "", fmt.Errorf("failed to get markdown style: %w", err)
	}

	// Get terminal width safely
	var screenWidth int
	if w, _, err := xterm.GetSize(int(os.Stdout.Fd())); err == nil {
		screenWidth = w
	} else {
		// Fallback to a reasonable default if we can't get the terminal width
		screenWidth = 80
	}

	// Create a new renderer with the specified style
	r, err := glamour.NewTermRenderer(
		// Use our custom style if available
		glamour.WithStylesFromJSONBytes(customStyle),
		glamour.WithWordWrap(screenWidth),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create markdown renderer: %w", err)
	}
	defer r.Close()

	out, err := r.Render(markdownText)
	if err != nil {
		return "", fmt.Errorf("failed to render markdown: %w", err)
	}

	return out, nil
}
