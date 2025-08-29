package utils

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/alecthomas/chroma/styles"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// DefaultHighlightSettings returns the default syntax highlighting settings
func DefaultHighlightSettings() *schema.SyntaxHighlighting {
	return &schema.SyntaxHighlighting{
		Enabled:                true,
		Formatter:              "terminal",
		Theme:                  "dracula",
		HighlightedOutputPager: true,
		LineNumbers:            true,
		Wrap:                   false,
	}
}

// getThemeAwareChromaTheme returns the appropriate Chroma theme based on the active Atmos theme
func getThemeAwareChromaTheme(config *schema.AtmosConfiguration) string {
	var themeName string

	// First priority: Check Viper for flags/env (includes ATMOS_THEME env var)
	if viper.IsSet("settings.terminal.theme") {
		themeName = viper.GetString("settings.terminal.theme")
	}

	// Second priority: Check config if available and non-nil
	if themeName == "" && config != nil &&
		config.Settings.Terminal.Theme != "" {
		themeName = config.Settings.Terminal.Theme
	}

	// Final fallback: Use default
	if themeName == "" {
		themeName = "default"
	}

	// Get the color scheme for the theme
	scheme, err := theme.GetColorSchemeForTheme(themeName)
	if err != nil || scheme == nil || scheme.ChromaTheme == "" {
		// Fallback to dracula if we can't get the theme's Chroma theme
		return "dracula"
	}

	return scheme.ChromaTheme
}

// GetHighlightSettings returns the syntax highlighting settings from the config or defaults
func GetHighlightSettings(config *schema.AtmosConfiguration) *schema.SyntaxHighlighting {
	defaults := DefaultHighlightSettings()
	if config.Settings.Terminal.SyntaxHighlighting == (schema.SyntaxHighlighting{}) {
		// Use theme-aware defaults
		defaults.Theme = getThemeAwareChromaTheme(config)
		return defaults
	}
	settings := &config.Settings.Terminal.SyntaxHighlighting

	// Apply defaults only for truly unset fields
	// NOTE: For proper tri-state handling, the schema should use *bool pointers
	// For now, we can't distinguish between explicitly set false and unset
	// So we only apply defaults for zero values that are likely unintended

	// For Enabled: We assume if the whole struct exists, they want it enabled unless explicitly false
	// This is the one case where false might be intentional, so we don't override it

	if settings.Formatter == "" {
		settings.Formatter = defaults.Formatter
	}
	if settings.Theme == "" {
		// Use theme-aware Chroma theme if not explicitly set
		settings.Theme = getThemeAwareChromaTheme(config)
	}

	// For these boolean fields, we only set defaults if they're false AND the config seems minimal
	// This is a compromise until the schema uses *bool
	// We check if other fields are set to determine if the user intentionally set these to false
	configHasExplicitSettings := settings.Formatter != "" || settings.Theme != ""

	if !settings.HighlightedOutputPager && !configHasExplicitSettings {
		settings.HighlightedOutputPager = defaults.HighlightedOutputPager
	}
	if !settings.LineNumbers && !configHasExplicitSettings {
		settings.LineNumbers = defaults.LineNumbers
	}
	if !settings.Wrap && !configHasExplicitSettings {
		settings.Wrap = defaults.Wrap
	}

	return settings
}

// HighlightCode highlights the given code using chroma with the specified lexer and theme
func HighlightCode(code string, lexerName string, theme string) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return code, nil
	}
	var buf bytes.Buffer
	err := quick.Highlight(&buf, code, lexerName, "terminal", theme)
	if err != nil {
		return code, err
	}
	return buf.String(), nil
}

var isTermPresent = term.IsTerminal(int(os.Stdout.Fd()))

// HighlightCodeWithConfig highlights the given code using the provided configuration.
func HighlightCodeWithConfig(config *schema.AtmosConfiguration, code string, format ...string) (string, error) {
	// Skip highlighting if not in a terminal or disabled
	if !isTermPresent || !GetHighlightSettings(config).Enabled {
		return code, nil
	}

	// Set terminal width
	config.Settings.Terminal.MaxWidth = templates.GetTerminalWidth()

	// Select lexer based on format or code content
	lexer := getLexer(format, code)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Select style
	settings := GetHighlightSettings(config)
	style := styles.Get(settings.Theme)
	if style == nil {
		style = styles.Fallback
	}

	// Select formatter
	formatter := getFormatter(settings)
	if formatter == nil {
		formatter = formatters.Fallback
	}

	// Format the code
	var buf bytes.Buffer
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code, err
	}
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return code, err
	}

	return buf.String(), nil
}

// getLexer selects a lexer based on format or code content.
func getLexer(format []string, code string) chroma.Lexer {
	if len(format) > 0 && format[0] != "" {
		return lexers.Get(strings.ToLower(format[0]))
	}
	trimmed := strings.TrimSpace(code)
	if json.Valid([]byte(trimmed)) {
		return lexers.Get("json")
	}
	if isYAML(trimmed) {
		return lexers.Get("yaml")
	}
	return lexers.Get("plaintext")
}

// isYAML checks if the code resembles YAML.
func isYAML(code string) bool {
	return (strings.Contains(code, ":") && !strings.HasPrefix(code, "{")) ||
		strings.Contains(code, "\n  ") ||
		strings.Contains(code, "\n- ")
}

// getFormatter selects a formatter based on settings.
func getFormatter(settings *schema.SyntaxHighlighting) chroma.Formatter {
	if settings.LineNumbers {
		return formatters.TTY256
	}
	return formatters.Get(settings.Formatter)
}

// HighlightWriter returns an io.Writer that highlights code written to it
type HighlightWriter struct {
	config schema.AtmosConfiguration
	writer io.Writer
	format string
}

// NewHighlightWriter creates a new HighlightWriter
func NewHighlightWriter(w io.Writer, config schema.AtmosConfiguration, format ...string) *HighlightWriter {
	var f string
	if len(format) > 0 {
		f = format[0]
	}
	return &HighlightWriter{
		config: config,
		writer: w,
		format: f,
	}
}

// Write implements io.Writer
// The returned byte count n is the length of p regardless of whether the highlighting
// process changes the actual number of bytes written to the underlying writer.
// This maintains compatibility with the io.Writer interface contract while still
// providing syntax highlighting functionality.
func (h *HighlightWriter) Write(p []byte) (n int, err error) {
	highlighted, err := HighlightCodeWithConfig(&h.config, string(p), h.format)
	if err != nil {
		return 0, err
	}

	// Write the highlighted content, ignoring the actual number of bytes written
	// since we'll return the original input length
	_, err = h.writer.Write([]byte(highlighted))
	if err != nil {
		// If there's an error, we can't be sure how many bytes were actually written
		return 0, err
	}

	// Return the original length of p as required by io.Writer interface
	// This ensures that the caller knows all bytes from p were processed
	return len(p), nil
}
