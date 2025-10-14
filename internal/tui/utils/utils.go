package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/arsham/figurine/figurine"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jwalton/go-supportscolor"
	xterm "golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/schema"
	mdstyle "github.com/cloudposse/atmos/pkg/ui/markdown"
)

// HighlightCode returns a syntax highlighted code for the specified language
func HighlightCode(code string, language string, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, code, language, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PrintStyledText prints a styled text to the terminal.
func PrintStyledText(text string) error {
	// Check if the terminal supports colors.
	// supportscolor automatically detects FORCE_COLOR, NO_COLOR, and other standard environment variables.
	if supportscolor.Stdout().SupportsColor {
		return figurine.Write(os.Stdout, text, "ANSI Regular.flf")
	}
	return nil
}

func PrintStyledTextToSpecifiedOutput(out io.Writer, text string) error {
	// Helper to check if a value is truthy
	// Truthy values: "1", "true" (case-insensitive) - standard Go bool values
	isTruthy := func(val string) bool {
		if val == "" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(val))
		return v == "1" || v == "true"
	}

	// Helper to check if a value is falsy
	// Falsy values: "0", "false" (case-insensitive) - standard Go bool values
	isFalsy := func(val string) bool {
		if val == "" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(val))
		return v == "0" || v == "false"
	}

	// Check if colors are explicitly disabled
	atmosForceColor := os.Getenv("ATMOS_FORCE_COLOR")
	cliColorForce := os.Getenv("CLICOLOR_FORCE")
	forceColorEnv := os.Getenv("FORCE_COLOR")
	noColor := os.Getenv("NO_COLOR")

	// If explicitly disabled, return early without printing
	if isFalsy(atmosForceColor) || isFalsy(cliColorForce) || isFalsy(forceColorEnv) || noColor != "" {
		return nil
	}

	// Check if colors are supported or forced
	forceColor := isTruthy(atmosForceColor) || isTruthy(cliColorForce) || isTruthy(forceColorEnv)
	if supportscolor.Stdout().SupportsColor || forceColor {
		// Write to the specified output writer, not os.Stdout
		return figurine.Write(out, text, "ANSI Regular.flf")
	}
	return nil
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
func NewAtmosHuhTheme() *huh.Theme {
	t := huh.ThemeCharm()
	cream := lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
	purple := lipgloss.AdaptiveColor{Light: "#5B00FF", Dark: "#5B00FF"}
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(cream).Background(purple)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(purple)
	t.Blurred.Title = t.Blurred.Title.Foreground(purple)
	return t
}
