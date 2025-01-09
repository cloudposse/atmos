package utils

import (
	"bytes"
	"io"
	"os"
	"strings"

	"encoding/json"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/alecthomas/chroma/styles"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"golang.org/x/term"
)

// DefaultHighlightSettings returns the default syntax highlighting settings
func DefaultHighlightSettings() *schema.SyntaxHighlighting {
	return &schema.SyntaxHighlighting{
		Enabled:     true,
		Formatter:   "terminal",
		Theme:       "dracula",
		UsePager:    true,
		LineNumbers: true,
		Wrap:        false,
	}
}

// GetHighlightSettings returns the syntax highlighting settings from the config or defaults
func GetHighlightSettings(config schema.AtmosConfiguration) *schema.SyntaxHighlighting {
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

// HighlightCodeWithConfig highlights the given code using the provided configuration
func HighlightCodeWithConfig(code string, config schema.AtmosConfiguration, format ...string) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return code, nil
	}
	settings := GetHighlightSettings(config)
	if !settings.Enabled {
		return code, nil
	}

	// Get terminal width
	config.Settings.Terminal.MaxWidth = templates.GetTerminalWidth()

	// Determine lexer based on format flag or content format
	var lexerName string
	if len(format) > 0 && format[0] != "" {
		// Use format flag if provided
		lexerName = strings.ToLower(format[0])
	} else {
		// This is just a fallback
		trimmed := strings.TrimSpace(code)

		// Try to parse as JSON first
		if json.Valid([]byte(trimmed)) {
			lexerName = "json"
		} else {
			// Check for common YAML indicators
			// 1. Contains key-value pairs with colons
			// 2. Does not start with a curly brace (which could indicate malformed JSON)
			// 3. Contains indentation or list markers
			if (strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "{")) ||
				strings.Contains(trimmed, "\n  ") ||
				strings.Contains(trimmed, "\n- ") {
				lexerName = "yaml"
			} else {
				// Fallback to plaintext if format is unclear
				lexerName = "plaintext"
			}
		}
	}

	// Get lexer
	lexer := lexers.Get(lexerName)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	// Get style
	s := styles.Get(settings.Theme)
	if s == nil {
		s = styles.Fallback
	}
	// Get formatter
	var formatter chroma.Formatter
	if settings.LineNumbers {
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
	highlighted, err := HighlightCodeWithConfig(string(p), h.config, h.format)
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
