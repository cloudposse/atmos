package utils

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/cloudposse/atmos/internal/tui/templates"
	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultHighlightSettings returns the default syntax highlighting settings
func DefaultHighlightSettings() *schema.SyntaxHighlighting {
	defer perf.Track(nil, "utils.DefaultHighlightSettings")()

	return &schema.SyntaxHighlighting{
		Enabled:     true,
		Formatter:   "terminal",
		Theme:       "dracula",
		LineNumbers: true,
		Wrap:        false,
	}
}

// GetHighlightSettings returns the syntax highlighting settings from the config or defaults
func GetHighlightSettings(config *schema.AtmosConfiguration) *schema.SyntaxHighlighting {
	defer perf.Track(config, "utils.GetHighlightSettings")()

	defaults := DefaultHighlightSettings()
	if config.Settings.Terminal.SyntaxHighlighting == (schema.SyntaxHighlighting{}) {
		return defaults
	}
	settings := &config.Settings.Terminal.SyntaxHighlighting
	// Apply defaults for any unset fields
	if !settings.Enabled {
		settings.Enabled = defaults.Enabled
	}
	if settings.Formatter == "" {
		settings.Formatter = defaults.Formatter
	}
	if settings.Theme == "" {
		settings.Theme = defaults.Theme
	}
	if !settings.LineNumbers {
		settings.LineNumbers = defaults.LineNumbers
	}
	if !settings.Wrap {
		settings.Wrap = defaults.Wrap
	}
	return settings
}

// HighlightCode highlights the given code using chroma with the specified lexer and theme
func HighlightCode(code string, lexerName string, theme string) (string, error) {
	defer perf.Track(nil, "utils.HighlightCode")()

	if !termUtils.IsTTYSupportForStdout() {
		return code, nil
	}
	var buf bytes.Buffer
	err := quick.Highlight(&buf, code, lexerName, "terminal", theme)
	if err != nil {
		return code, err
	}
	return buf.String(), nil
}

// HighlightCodeWithConfig highlights the given code using the provided configuration.
func HighlightCodeWithConfig(config *schema.AtmosConfiguration, code string, format ...string) (string, error) {
	defer perf.Track(config, "utils.HighlightCodeWithConfig")()

	// Return plain code if config is nil
	if config == nil {
		return code, nil
	}

	// Check if stdout is a terminal. Only stdout matters because highlighted
	// output goes to the data channel (stdout). Stderr may still be a TTY when
	// stdout is piped, so checking it would defeat pipe detection.
	// Note: This must be checked dynamically, not at package init time.
	// Some environments (like VHS) set up the TTY after the binary is loaded.
	isTerm := termUtils.IsTTYSupportForStdout()

	// Check if color is forced via ForceColor (ATMOS_FORCE_COLOR).
	// ForceColor acts like isTTY=true for color support checking.
	forceColor := config.Settings.Terminal.ForceColor

	// Skip highlighting if not in a terminal AND color is not forced.
	// The Color setting enables color when TTY is present but doesn't force it.
	// ForceColor (ATMOS_FORCE_COLOR) explicitly enables highlighting even without TTY.
	if !isTerm && !forceColor {
		return code, nil
	}

	// Check if color is explicitly disabled via NoColor.
	// NoColor takes precedence over everything.
	if config.Settings.Terminal.NoColor {
		return code, nil
	}

	// Skip highlighting if syntax highlighting is disabled in settings.
	if !GetHighlightSettings(config).Enabled {
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
	defer perf.Track(nil, "utils.getLexer")()

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
