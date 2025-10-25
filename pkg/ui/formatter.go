package ui

import (
	"fmt"
	stdio "io"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var (
	// Global formatter instance (like the logger).
	globalFormatter Formatter
	formatterMu     sync.RWMutex
)

// InitFormatter initializes the global formatter with an I/O context.
// This should be called once at application startup (in root.go).
func InitFormatter(ioCtx io.Context) {
	formatterMu.Lock()
	defer formatterMu.Unlock()
	globalFormatter = NewFormatter(ioCtx)
	Format = globalFormatter // Also expose for advanced use
}

// getFormatter returns the global formatter instance.
// Panics if not initialized (programming error, not runtime error).
func getFormatter() Formatter {
	formatterMu.RLock()
	defer formatterMu.RUnlock()
	if globalFormatter == nil {
		panic("ui.InitFormatter() must be called before using UI functions")
	}
	return globalFormatter
}

// Package-level functions that delegate to the global formatter.

// Data writes plain text to stdout (data channel).
// Use this for JSON, YAML, or other machine-readable output.
func Data(content string) error {
	_, err := fmt.Fprint(getFormatter().(*formatter).ioCtx.Data(), content)
	return err
}

// Dataf writes formatted text to stdout (data channel).
// Use this for formatted data output.
func Dataf(format string, a ...interface{}) error {
	_, err := fmt.Fprintf(getFormatter().(*formatter).ioCtx.Data(), format, a...)
	return err
}

// Markdown writes rendered markdown to stdout (data channel).
// Use this for help text, documentation, and other pipeable content.
func Markdown(content string) error {
	return getFormatter().Markdown(content, true)
}

// MarkdownMessage writes rendered markdown to stderr (UI channel).
// Use this for formatted UI messages and errors.
func MarkdownMessage(content string) error {
	return getFormatter().Markdown(content, false)
}

// Success writes a success message with green checkmark to stderr (UI channel).
func Success(text string) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Success(text))
	return err
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
func Successf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Successf(format, a...))
	return err
}

// Error writes an error message with red X to stderr (UI channel).
func Error(text string) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Error(text))
	return err
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
func Errorf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Errorf(format, a...))
	return err
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
func Warning(text string) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Warning(text))
	return err
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
func Warningf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Warningf(format, a...))
	return err
}

// Info writes an info message with cyan info icon to stderr (UI channel).
func Info(text string) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Info(text))
	return err
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
func Infof(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	_, err := fmt.Fprintln(f.ioCtx.UI(), f.Infof(format, a...))
	return err
}

// Format exposes the global formatter for advanced use cases.
// Most code should use the package-level functions (ui.Success, ui.Error, etc.).
// Use this when you need the formatted string without writing it.
var Format Formatter

// formatter implements the Formatter interface.
type formatter struct {
	ioCtx  io.Context
	styles *StyleSet
}

// NewFormatter creates a new Formatter that uses io.Terminal for capabilities.
// Most code should use the package-level functions instead (ui.Markdown, ui.Success, etc.).
func NewFormatter(ioCtx io.Context) Formatter {
	styles := generateStyleSet(ioCtx.Terminal().ColorProfile())

	return &formatter{
		ioCtx:  ioCtx,
		styles: styles,
	}
}

// newFormatter is the internal constructor (kept for backward compatibility).
func newFormatter(ioCtx io.Context) Formatter {
	return NewFormatter(ioCtx)
}

func (f *formatter) Styles() *StyleSet {
	return f.styles
}

func (f *formatter) SupportsColor() bool {
	return f.ioCtx.Terminal().ColorProfile() != io.ColorNone
}

func (f *formatter) ColorProfile() io.ColorProfile {
	return f.ioCtx.Terminal().ColorProfile()
}

// StatusMessage formats a message with an icon and color.
// This is the foundational method used by Success, Error, Warning, and Info.
//
// Parameters:
//   - icon: The icon/symbol to prefix the message (e.g., "✓", "✗", "⚠", "ℹ")
//   - style: The lipgloss style to apply (determines color)
//   - text: The message text
//
// Returns formatted string: "{icon} {text}" with color applied (or plain if no color support).
func (f *formatter) StatusMessage(icon string, style lipgloss.Style, text string) string {
	if !f.SupportsColor() {
		return fmt.Sprintf("%s %s", icon, text)
	}
	return style.Render(fmt.Sprintf("%s %s", icon, text))
}

// Semantic formatting - delegates to StatusMessage with appropriate icons and styles.
func (f *formatter) Success(text string) string {
	return f.StatusMessage("✓", f.styles.Success, text)
}

func (f *formatter) Successf(format string, a ...interface{}) string {
	return f.Success(fmt.Sprintf(format, a...))
}

func (f *formatter) Warning(text string) string {
	return f.StatusMessage("⚠", f.styles.Warning, text)
}

func (f *formatter) Warningf(format string, a ...interface{}) string {
	return f.Warning(fmt.Sprintf(format, a...))
}

func (f *formatter) Error(text string) string {
	return f.StatusMessage("✗", f.styles.Error, text)
}

func (f *formatter) Errorf(format string, a ...interface{}) string {
	return f.Error(fmt.Sprintf(format, a...))
}

func (f *formatter) Info(text string) string {
	return f.StatusMessage("ℹ", f.styles.Info, text)
}

func (f *formatter) Infof(format string, a ...interface{}) string {
	return f.Info(fmt.Sprintf(format, a...))
}

func (f *formatter) Muted(text string) string {
	if !f.SupportsColor() {
		return text
	}
	return f.styles.Muted.Render(text)
}

func (f *formatter) Bold(text string) string {
	if !f.SupportsColor() {
		return text
	}
	return f.styles.Title.Render(text)
}

func (f *formatter) Heading(text string) string {
	if !f.SupportsColor() {
		return text
	}
	return f.styles.Heading.Render(text)
}

func (f *formatter) Label(text string) string {
	if !f.SupportsColor() {
		return text
	}
	return f.styles.Label.Render(text)
}

// Markdown writes rendered markdown to the appropriate channel.
// useDataChannel=true writes to stdout (for pipeable content like help).
// useDataChannel=false writes to stderr (for UI messages).
// Degrades gracefully to plain text if rendering fails.
func (f *formatter) Markdown(content string, useDataChannel bool) error {
	rendered, _ := f.RenderMarkdown(content)

	var writer stdio.Writer
	if useDataChannel {
		writer = f.ioCtx.Data()
	} else {
		writer = f.ioCtx.UI()
	}

	_, err := fmt.Fprint(writer, rendered)
	return err
}

// RenderMarkdown is a low-level function that returns the rendered markdown string.
// Most code should use Markdown() instead, which writes directly to the channel.
// This is kept for backward compatibility and advanced use cases.
func (f *formatter) RenderMarkdown(content string) (string, error) {
	// Determine max width from config or terminal
	maxWidth := f.ioCtx.Config().AtmosConfig.Settings.Terminal.MaxWidth
	if maxWidth == 0 {
		// Use terminal width if available
		termWidth := f.ioCtx.Terminal().Width(io.StreamOutput)
		if termWidth > 0 {
			maxWidth = termWidth
		}
	}

	// Build glamour options based on color profile
	var opts []glamour.TermRendererOption

	if maxWidth > 0 {
		opts = append(opts, glamour.WithWordWrap(maxWidth))
	}

	// Select style based on color profile
	var styleName string
	switch f.ioCtx.Terminal().ColorProfile() {
	case io.ColorNone:
		styleName = "notty"
	case io.Color16, io.Color256, io.ColorTrue:
		// Use dark style as default - this will be theme-aware in PR #1433
		styleName = "dark"
	}

	if styleName != "" {
		opts = append(opts, glamour.WithStylePath(styleName))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		// Degrade gracefully: return plain content if renderer creation fails
		return content, nil
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		// Degrade gracefully: return plain content if rendering fails
		return content, nil
	}

	return rendered, nil
}

// generateStyleSet creates a StyleSet based on color profile.
// This is a simplified version - will be replaced by theme system from PR #1433.
func generateStyleSet(profile io.ColorProfile) *StyleSet {
	if profile == io.ColorNone {
		// No color - return styles with no formatting
		return &StyleSet{
			Title:   lipgloss.NewStyle(),
			Heading: lipgloss.NewStyle(),
			Body:    lipgloss.NewStyle(),
			Muted:   lipgloss.NewStyle(),
			Success: lipgloss.NewStyle(),
			Warning: lipgloss.NewStyle(),
			Error:   lipgloss.NewStyle(),
			Info:    lipgloss.NewStyle(),
			Link:    lipgloss.NewStyle(),
			Command: lipgloss.NewStyle(),
			Label:   lipgloss.NewStyle(),
		}
	}

	// Use existing theme.Styles for now
	// This will be replaced with full theme system from PR #1433
	return &StyleSet{
		Title:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.ColorCyan)),
		Heading: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBlue)),
		Body:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorWhite)),
		Muted:   theme.Styles.GrayText,
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen)),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange)),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed)),
		Info:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)),
		Link:    theme.Styles.Link,
		Command: theme.Styles.CommandName,
		Label:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.ColorBlue)),
	}
}
