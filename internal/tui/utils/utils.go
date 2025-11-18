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
	"github.com/jwalton/go-supportscolor"
	"github.com/spf13/viper"
	xterm "golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	mdstyle "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// AnsiRegularFont is the figurine font used for styled ASCII text.
	AnsiRegularFont = "ANSI Regular.flf"
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

// PrintStyledText prints a styled text to the terminal.
func PrintStyledText(text string) error {
	// Check NO_COLOR first (highest priority).
	if os.Getenv("NO_COLOR") != "" { //nolint:forbidigo // Standard terminal env var
		return nil
	}

	// Check --force-color flag (via Viper).
	// This allows `atmos version --force-color` to work for screenshot generation.
	if viper.GetBool("force-color") {
		return figurine.Write(os.Stdout, text, AnsiRegularFont)
	}

	// Check standard CLICOLOR_FORCE and FORCE_COLOR env vars.
	if os.Getenv("CLICOLOR_FORCE") != "" || os.Getenv("FORCE_COLOR") != "" { //nolint:forbidigo // Standard terminal env vars
		return figurine.Write(os.Stdout, text, AnsiRegularFont)
	}

	// Fall back to automatic color detection.
	// supportscolor automatically detects TTY and other standard environment variables.
	if supportscolor.Stdout().SupportsColor {
		return figurine.Write(os.Stdout, text, AnsiRegularFont)
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

	// Bind environment variables for color control.
	_ = viper.BindEnv("ATMOS_FORCE_COLOR")
	_ = viper.BindEnv("CLICOLOR_FORCE")
	_ = viper.BindEnv("FORCE_COLOR")
	_ = viper.BindEnv("NO_COLOR")

	// Check if colors are explicitly disabled.
	atmosForceColor := viper.GetString("ATMOS_FORCE_COLOR")
	cliColorForce := viper.GetString("CLICOLOR_FORCE")
	forceColorEnv := viper.GetString("FORCE_COLOR")
	noColor := viper.GetString("NO_COLOR")

	// If explicitly disabled, return early without printing
	if isFalsy(atmosForceColor) || isFalsy(cliColorForce) || isFalsy(forceColorEnv) || noColor != "" {
		return nil
	}

	// Check if colors are supported or forced
	forceColor := isTruthy(atmosForceColor) || isTruthy(cliColorForce) || isTruthy(forceColorEnv)
	if supportscolor.Stdout().SupportsColor || forceColor {
		// Write to the specified output writer, not os.Stdout
		return figurine.Write(out, text, AnsiRegularFont)
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
