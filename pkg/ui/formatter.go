package ui

import (
	"fmt"
	stdio "io"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Character constants.
	newline = "\n"
	tab     = "\t"
)

var (
	// Global formatter and terminal instances.
	globalFormatter Formatter
	globalTerminal  terminal.Terminal
	formatterMu     sync.RWMutex
)

// InitFormatter initializes the global formatter with an I/O context.
// This should be called once at application startup (in root.go).
func InitFormatter(ioCtx io.Context) {
	formatterMu.Lock()
	defer formatterMu.Unlock()

	// Create adapter for terminal to write through I/O layer
	termWriter := io.NewTerminalWriter(ioCtx)

	// Create terminal instance with I/O writer for automatic masking
	// terminal.Write() → io.Write(UIStream) → masking → stderr
	globalTerminal = terminal.New(terminal.WithIO(termWriter))

	// Create formatter with I/O context and terminal
	globalFormatter = NewFormatter(ioCtx, globalTerminal)
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
// Flow: ui.Success() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Success(text string) error {
	f := getFormatter().(*formatter)
	formatted := f.Success(text) + newline
	return f.terminal.Write(formatted)
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
// Flow: ui.Successf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Successf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	formatted := f.Successf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Error writes an error message with red X to stderr (UI channel).
// Flow: ui.Error() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Error(text string) error {
	f := getFormatter().(*formatter)
	formatted := f.Error(text) + newline
	return f.terminal.Write(formatted)
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
// Flow: ui.Errorf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Errorf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	formatted := f.Errorf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warning() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warning(text string) error {
	f := getFormatter().(*formatter)
	formatted := f.Warning(text) + newline
	return f.terminal.Write(formatted)
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warningf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warningf(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	formatted := f.Warningf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Info writes an info message with cyan info icon to stderr (UI channel).
// Flow: ui.Info() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Info(text string) error {
	f := getFormatter().(*formatter)
	formatted := f.Info(text) + newline
	return f.terminal.Write(formatted)
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
// Flow: ui.Infof() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Infof(format string, a ...interface{}) error {
	f := getFormatter().(*formatter)
	formatted := f.Infof(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Format exposes the global formatter for advanced use cases.
// Most code should use the package-level functions (ui.Success, ui.Error, etc.).
// Use this when you need the formatted string without writing it.
var Format Formatter

// formatter implements the Formatter interface.
type formatter struct {
	ioCtx    io.Context
	terminal terminal.Terminal
	styles   *StyleSet
}

// NewFormatter creates a new Formatter with I/O context and terminal.
// Most code should use the package-level functions instead (ui.Markdown, ui.Success, etc.).
func NewFormatter(ioCtx io.Context, term terminal.Terminal) Formatter {
	styles := generateStyleSet(term.ColorProfile())

	return &formatter{
		ioCtx:    ioCtx,
		terminal: term,
		styles:   styles,
	}
}

func (f *formatter) Styles() *StyleSet {
	return f.styles
}

func (f *formatter) SupportsColor() bool {
	return f.terminal.ColorProfile() != terminal.ColorNone
}

func (f *formatter) ColorProfile() terminal.ColorProfile {
	return f.terminal.ColorProfile()
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
func (f *formatter) StatusMessage(icon string, style *lipgloss.Style, text string) string {
	if !f.SupportsColor() {
		return fmt.Sprintf("%s %s", icon, text)
	}
	return style.Render(fmt.Sprintf("%s %s", icon, text))
}

// Semantic formatting - delegates to StatusMessage with appropriate icons and styles.
func (f *formatter) Success(text string) string {
	return f.StatusMessage("✓", &f.styles.Success, text)
}

func (f *formatter) Successf(format string, a ...interface{}) string {
	return f.Success(fmt.Sprintf(format, a...))
}

func (f *formatter) Warning(text string) string {
	return f.StatusMessage("⚠", &f.styles.Warning, text)
}

func (f *formatter) Warningf(format string, a ...interface{}) string {
	return f.Warning(fmt.Sprintf(format, a...))
}

func (f *formatter) Error(text string) string {
	return f.StatusMessage("✗", &f.styles.Error, text)
}

func (f *formatter) Errorf(format string, a ...interface{}) string {
	return f.Error(fmt.Sprintf(format, a...))
}

func (f *formatter) Info(text string) string {
	return f.StatusMessage("ℹ", &f.styles.Info, text)
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
// UseDataChannel=true writes to stdout (for pipeable content like help).
// UseDataChannel=false writes to stderr (for UI messages).
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
		termWidth := f.terminal.Width(terminal.Stdout)
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
	styleName := f.selectMarkdownStyle()
	if styleName != "" {
		opts = append(opts, glamour.WithStylePath(styleName))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		// Degrade gracefully: return plain content if renderer creation fails
		return content, err
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		// Degrade gracefully: return plain content if rendering fails
		return content, err
	}

	return rendered, nil
}

// selectMarkdownStyle returns the glamour style name based on terminal color profile.
// This will be replaced with full theme system from PR #1433.
func (f *formatter) selectMarkdownStyle() string {
	switch f.terminal.ColorProfile() {
	case terminal.ColorNone:
		return "notty"
	case terminal.Color16, terminal.Color256, terminal.ColorTrue:
		// Use dark style as default - this will be theme-aware in PR #1433
		return "dark"
	default:
		return ""
	}
}

// generateStyleSet creates a StyleSet based on color profile.
// This is a simplified version - will be replaced by theme system from PR #1433.
func generateStyleSet(profile terminal.ColorProfile) *StyleSet {
	if profile == terminal.ColorNone {
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
