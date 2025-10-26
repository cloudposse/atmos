package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/io"
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
	StatusMessage(icon string, style lipgloss.Style, text string) string

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

	// Markdown rendering - writes rendered markdown to channel (degrades gracefully to plain text)
	// Automatically chooses Data or UI channel based on useDataChannel parameter
	Markdown(content string, useDataChannel bool) error

	// Theme access
	Styles() *StyleSet // Access to full StyleSet

	// Capability queries (delegates to terminal.Terminal)
	ColorProfile() terminal.ColorProfile
	SupportsColor() bool

	// Low-level: Returns rendered markdown string (for advanced use only)
	// Most code should use Markdown() instead
	RenderMarkdown(content string) (string, error)
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

// Output provides high-level output methods with formatting.
// DEPRECATED: Use io.Context.Data()/UI() + ui.Formatter directly instead.
//
// Old pattern (being phased out):
//
//	out.Success("done!")  // Where does this go? Not explicit
//
// New pattern (preferred):
//
//	io := io.Context
//	ui := ui.Formatter
//	fmt.Fprintf(io.UI(), "%s\n", ui.Success("done!"))  // Explicit channel
//
// This interface exists for backward compatibility during migration.
type Output interface {
	// Data output (to stdout - pipeable)
	// DEPRECATED: Use fmt.Fprintf(io.Data(), ...) instead
	Print(a ...interface{})
	Printf(format string, a ...interface{})
	Println(a ...interface{})

	// UI output (to stderr - human-readable)
	// DEPRECATED: Use fmt.Fprintf(io.UI(), ui.Success(...)) instead
	Message(format string, a ...interface{})
	Success(format string, a ...interface{})
	Warning(format string, a ...interface{})
	Error(format string, a ...interface{})
	Info(format string, a ...interface{})

	// Formatted output
	// DEPRECATED: Use fmt.Fprint(io.Data(), ui.RenderMarkdown(...)) instead
	Markdown(content string) error   // Rendered to stdout
	MarkdownUI(content string) error // Rendered to stderr

	// Output options
	SetTrimTrailingWhitespace(enabled bool) // Enable/disable trailing whitespace trimming
	TrimTrailingWhitespace() bool           // Get current trimming setting

	// Get underlying components
	Formatter() Formatter
	IOContext() io.Context // Access to underlying I/O
}
