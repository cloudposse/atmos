package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/terminal"
)

// Formatter provides text formatting with automatic degradation.
// Key Principle: Formatter RETURNS FORMATTED STRINGS - it never writes to streams.
//
// Usage Pattern:
//
//	io := cmd.Context().Value(ioContextKey).(io.Context)
//	ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)
//
//	// Format text with automatic icons
//	msg := ui.Success("Deployment complete!")  // Returns "✓ Deployment complete!" in green
//
//	// Developer chooses channel
//	fmt.Fprintf(io.UI(), "%s\n", msg)  // UI message → stderr
//
// Uses io.Terminal for capability detection and theme.StyleSet for styling.
type Formatter interface {
	// Status message formatting - standardized output with icons
	// This is the foundational method used by Success/Error/Warning/Info
	StatusMessage(icon string, style *lipgloss.Style, text string) string

	// Semantic formatting - returns styled strings with automatic icons (uses theme.StyleSet)
	// These methods use StatusMessage internally with predefined icons
	Success(text string) string                      // Returns "✓ {text}" in green
	Successf(format string, a ...interface{}) string // Returns "✓ {formatted}" in green
	Warning(text string) string                      // Returns "⚠ {text}" in yellow
	Warningf(format string, a ...interface{}) string // Returns "⚠ {formatted}" in yellow
	Error(text string) string                        // Returns "✗ {text}" in red
	Errorf(format string, a ...interface{}) string   // Returns "✗ {formatted}" in red
	Info(text string) string                         // Returns "ℹ {text}" in cyan
	Infof(format string, a ...interface{}) string    // Returns "ℹ {formatted}" in cyan
	Muted(text string) string                        // Returns muted text (gray, no icon)

	// Text formatting - returns styled strings
	Bold(text string) string    // Returns bold text
	Heading(text string) string // Returns heading-styled text
	Label(text string) string   // Returns label-styled text

	// Theme access
	Styles() *StyleSet // Access to full StyleSet

	// Capability queries (delegates to terminal.Terminal)
	ColorProfile() terminal.ColorProfile
	SupportsColor() bool

	// Markdown rendering - returns rendered markdown string (pure function, no I/O)
	// For writing markdown to channels, use package-level ui.Markdown() or ui.MarkdownMessage()
	Markdown(content string) (string, error)
}

// StyleSet provides pre-configured lipgloss styles for common UI elements.
// This is a simplified version that will be replaced by PR #1433's full theme system.
type StyleSet struct {
	// Text styles
	Title   lipgloss.Style
	Heading lipgloss.Style
	Body    lipgloss.Style
	Muted   lipgloss.Style

	// Status styles
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style

	// UI element styles
	Link    lipgloss.Style
	Command lipgloss.Style
	Label   lipgloss.Style
}
