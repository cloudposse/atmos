package utils

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/cloudposse/atmos/internal/tui/templates"
	termWriter "github.com/cloudposse/atmos/internal/tui/templates/term"
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

	if !termWriter.IsTTYSupportForStdout() {
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

	// Check if either stdout or stderr is a terminal (provenance goes to stderr)
	isTerm := termWriter.IsTTYSupportForStdout() || termWriter.IsTTYSupportForStderr()

	// Skip highlighting if not in a terminal or disabled
	if !isTerm || !GetHighlightSettings(config).Enabled {
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

// HighlightWriter returns an io.Writer that highlights code written to it
type HighlightWriter struct {
	config schema.AtmosConfiguration
	writer io.Writer
	format string
}

// NewHighlightWriter creates a new HighlightWriter
func NewHighlightWriter(w io.Writer, config schema.AtmosConfiguration, format ...string) *HighlightWriter {
	defer perf.Track(&config, "utils.NewHighlightWriter")()

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
