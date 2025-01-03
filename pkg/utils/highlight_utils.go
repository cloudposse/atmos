package utils

import (
	"bytes"
	"io"
	"os"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/alecthomas/chroma/styles"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultHighlightSettings returns the default syntax highlighting settings
func DefaultHighlightSettings() *schema.SyntaxHighlightingSettings {
	return &schema.SyntaxHighlightingSettings{
		Enabled:   true,
		Lexer:     "yaml",
		Formatter: "terminal",
		Style:     "dracula",
		Options: &schema.SyntaxHighlightOptions{
			LineNumbers: false,
			Wrap:        false,
		},
	}
}

// GetHighlightSettings returns the syntax highlighting settings from the config or defaults
func GetHighlightSettings(config schema.AtmosConfiguration) *schema.SyntaxHighlightingSettings {
	defaults := DefaultHighlightSettings()

	if config.Settings == nil {
		return defaults
	}

	if config.Settings.Terminal == nil {
		config.Settings.Terminal = &schema.TerminalSettings{
			SyntaxHighlighting: defaults,
		}
	}

	if config.Settings.Terminal.SyntaxHighlighting == nil {
		config.Settings.Terminal.SyntaxHighlighting = defaults
		return defaults
	}

	settings := config.Settings.Terminal.SyntaxHighlighting

	// Apply defaults for any unset fields
	if !settings.Enabled {
		settings.Enabled = defaults.Enabled
	}
	if settings.Lexer == "" {
		settings.Lexer = defaults.Lexer
	}
	if settings.Formatter == "" {
		settings.Formatter = defaults.Formatter
	}
	if settings.Style == "" {
		settings.Style = defaults.Style
	}
	if settings.Options == nil {
		settings.Options = defaults.Options
	}

	return settings
}

// HighlightCode highlights the given code using chroma with the specified lexer and style
func HighlightCode(code string, lexerName string, style string) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return code, nil
	}

	var buf bytes.Buffer
	err := quick.Highlight(&buf, code, lexerName, "terminal", style)
	if err != nil {
		return code, err
	}

	return buf.String(), nil
}

// HighlightCodeWithConfig highlights the given code using the provided configuration
func HighlightCodeWithConfig(code string, config schema.AtmosConfiguration) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return code, nil
	}

	settings := GetHighlightSettings(config)
	if !settings.Enabled {
		return code, nil
	}

	// Get lexer
	lexer := lexers.Get(settings.Lexer)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Get style
	s := styles.Get(settings.Style)
	if s == nil {
		s = styles.Fallback
	}

	// Get formatter
	var formatter chroma.Formatter
	if settings.Options != nil && settings.Options.LineNumbers {
		formatter = formatters.TTY256
	} else {
		formatter = formatters.Get(settings.Formatter)
		if formatter == nil {
			formatter = formatters.Fallback
		}
	}

	// Create buffer for output
	var buf bytes.Buffer

	// Format the code
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code, err
	}

	err = formatter.Format(&buf, s, iterator)
	if err != nil {
		return code, err
	}

	return buf.String(), nil
}

// HighlightWriter returns an io.Writer that highlights code written to it
type HighlightWriter struct {
	config schema.AtmosConfiguration
	writer io.Writer
}

// NewHighlightWriter creates a new HighlightWriter
func NewHighlightWriter(w io.Writer, config schema.AtmosConfiguration) *HighlightWriter {
	return &HighlightWriter{
		config: config,
		writer: w,
	}
}

// Write implements io.Writer
func (h *HighlightWriter) Write(p []byte) (n int, err error) {
	highlighted, err := HighlightCodeWithConfig(string(p), h.config)
	if err != nil {
		return 0, err
	}
	return h.writer.Write([]byte(highlighted))
}
