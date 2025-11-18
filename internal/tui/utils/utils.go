package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/arsham/figurine/figurine"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/jwalton/go-supportscolor"
	xterm "golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	mdstyle "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// WriteJSON writes JSON to stdout (data channel).
// This is a convenience wrapper around data.WriteJSON().
func WriteJSON(v interface{}) error {
	return data.WriteJSON(v)
}

// WriteYAML writes YAML to stdout (data channel).
// This is a convenience wrapper around data.WriteYAML().
func WriteYAML(v interface{}) error {
	return data.WriteYAML(v)
}

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

func PrintStyledTextToSpecifiedOutput(out io.Writer, text string) error {
	return figurine.Write(out, text, "ANSI Regular.flf")
}

// RenderMarkdown renders markdown text with terminal styling
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

// NewAtmosHuhTheme returns the Atmos-styled Huh theme for interactive prompts.
// Uses the current theme's color scheme from pkg/ui/theme for consistency.
func NewAtmosHuhTheme() *huh.Theme {
	t := huh.ThemeCharm()

	// Get current theme styles for consistent colors.
	styles := theme.GetCurrentStyles()

	// Extract colors from theme for interactive elements.
	buttonForeground := styles.Interactive.ButtonForeground.GetForeground()
	buttonBackground := styles.Interactive.ButtonBackground.GetBackground()
	primaryColor := styles.Selected.GetForeground()

	// Use theme's colors for interactive elements.
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(buttonForeground).
		Background(buttonBackground)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(primaryColor)
	t.Blurred.Title = t.Blurred.Title.Foreground(primaryColor)

	return t
}
