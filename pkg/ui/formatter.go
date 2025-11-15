package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Character constants.
	newline = "\n"
	tab     = "\t"

	// Format templates.
	iconMessageFormat = "%s %s"
)

var (
	// Global formatter instance and I/O context.
	globalIO        io.Context
	globalFormatter *formatter
	formatterMu     sync.RWMutex
)

// InitFormatter initializes the global formatter with an I/O context.
// This should be called once at application startup (in root.go).
func InitFormatter(ioCtx io.Context) {
	formatterMu.Lock()
	defer formatterMu.Unlock()

	// Store I/O context for package-level output functions
	globalIO = ioCtx

	// Create adapter for terminal to write through I/O layer
	termWriter := io.NewTerminalWriter(ioCtx)

	// Create terminal instance with I/O writer for automatic masking
	// terminal.Write() → io.Write(UIStream) → masking → stderr
	globalTerminal := terminal.New(terminal.WithIO(termWriter))

	// Create formatter with I/O context and terminal
	globalFormatter = NewFormatter(ioCtx, globalTerminal).(*formatter)
	Format = globalFormatter // Also expose for advanced use
}

// getFormatter returns the global formatter instance.
// Returns error if not initialized instead of panicking.
func getFormatter() (*formatter, error) {
	formatterMu.RLock()
	defer formatterMu.RUnlock()
	if globalFormatter == nil {
		return nil, errUtils.ErrUIFormatterNotInitialized
	}
	return globalFormatter, nil
}

// Package-level functions that delegate to the global formatter.

// Toast writes a toast notification with a custom icon and message to stderr (UI channel).
// This is the primary pattern for toast-style notifications with flexible icon support.
// Supports multiline messages - automatically splits on newlines and indents continuation lines.
// Flow: ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
//
// Parameters:
//   - icon: Custom icon/emoji (e.g., "📦", "🔧", "✓", or use theme.Styles.Checkmark.String())
//   - message: The message text (can contain newlines for multiline toasts)
//
// Example usage:
//
//	ui.Toast("📦", "Using latest version: 1.2.3")
//	ui.Toast("🔧", "Tool not installed")
//	ui.Toast("✓", "Installation complete\nVersion: 1.2.3\nLocation: /usr/local/bin")
func Toast(icon, message string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.formatToast(icon, message)
	return f.terminal.Write(formatted)
}

// Toastf writes a formatted toast notification with a custom icon to stderr (UI channel).
// This is the primary pattern for formatted toast-style notifications with flexible icon support.
// Flow: ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
//
// Parameters:
//   - icon: Custom icon/emoji (e.g., "📦", "🔧", "✓", or use theme.Styles.Checkmark.String())
//   - format: Printf-style format string
//   - a: Format arguments
//
// Example usage:
//
//	ui.Toastf("📦", "Using latest version: %s", version)
//	ui.Toastf("🔧", "Tool %s is not installed", toolName)
//	ui.Toastf(theme.Styles.Checkmark.String(), "Installed %s/%s@%s", owner, repo, version)
func Toastf(icon, format string, a ...interface{}) error {
	message := fmt.Sprintf(format, a...)
	return Toast(icon, message)
}

// Markdown writes rendered markdown to stdout (data channel).
// Use this for help text, documentation, and other pipeable formatted content.
// Note: Delegates to globalFormatter.Markdown() for rendering, then writes to data channel.
func Markdown(content string) error {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalFormatter == nil || globalIO == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}

	rendered, err := globalFormatter.Markdown(content)
	if err != nil {
		// Degrade gracefully - write plain content if rendering fails
		rendered = content
	}

	_, writeErr := fmt.Fprint(globalIO.Data(), rendered)
	return writeErr
}

// Markdownf writes formatted markdown to stdout (data channel).
func Markdownf(format string, a ...interface{}) error {
	content := fmt.Sprintf(format, a...)
	return Markdown(content)
}

// MarkdownMessage writes rendered markdown to stderr (UI channel).
// Use this for formatted UI messages and errors.
func MarkdownMessage(content string) error {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalFormatter == nil || globalIO == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}

	rendered, err := globalFormatter.Markdown(content)
	if err != nil {
		// Degrade gracefully - write plain content if rendering fails
		rendered = content
	}

	_, writeErr := fmt.Fprint(globalIO.UI(), rendered)
	return writeErr
}

// MarkdownMessagef writes formatted markdown to stderr (UI channel).
func MarkdownMessagef(format string, a ...interface{}) error {
	content := fmt.Sprintf(format, a...)
	return MarkdownMessage(content)
}

// Success writes a success message with green checkmark to stderr (UI channel).
// This is a convenience wrapper around Toast() with themed success icon and color.
// Flow: ui.Success() → ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Success(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored success message, then write via terminal
	formatted := f.Success(text) + newline
	return f.terminal.Write(formatted)
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
// This is a convenience wrapper around Toastf() with themed success icon and color.
// Flow: ui.Successf() → ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Successf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored success message, then write via terminal
	formatted := f.Successf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Error writes an error message with red X to stderr (UI channel).
// This is a convenience wrapper around Toast() with themed error icon and color.
// Flow: ui.Error() → ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Error(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored error message, then write via terminal
	formatted := f.Error(text) + newline
	return f.terminal.Write(formatted)
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
// This is a convenience wrapper around Toastf() with themed error icon and color.
// Flow: ui.Errorf() → ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Errorf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored error message, then write via terminal
	formatted := f.Errorf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
// This is a convenience wrapper around Toast() with themed warning icon and color.
// Flow: ui.Warning() → ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warning(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored warning message, then write via terminal
	formatted := f.Warning(text) + newline
	return f.terminal.Write(formatted)
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
// This is a convenience wrapper around Toastf() with themed warning icon and color.
// Flow: ui.Warningf() → ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warningf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored warning message, then write via terminal
	formatted := f.Warningf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Info writes an info message with cyan info icon to stderr (UI channel).
// This is a convenience wrapper around Toast() with themed info icon and color.
// Flow: ui.Info() → ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Info(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored info message, then write via terminal
	formatted := f.Info(text) + newline
	return f.terminal.Write(formatted)
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
// This is a convenience wrapper around Toastf() with themed info icon and color.
// Flow: ui.Infof() → ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Infof(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	// Use formatter to get colored info message, then write via terminal
	formatted := f.Infof(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Write writes plain text to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Write(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	return f.terminal.Write(text)
}

// Writef writes formatted text to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Writef() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Writef(format string, a ...interface{}) error {
	return Write(fmt.Sprintf(format, a...))
}

// Writeln writes text followed by a newline to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Writeln() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Writeln(text string) error {
	return Write(text + newline)
}

// Format exposes the global formatter for advanced use cases.
// Most code should use the package-level functions (ui.Success, ui.Error, etc.).
// Use this when you need the formatted string without writing it.
var Format Formatter

// formatter implements the Formatter interface.
type formatter struct {
	ioCtx    io.Context
	terminal terminal.Terminal
	styles   *theme.StyleSet
}

// NewFormatter creates a new Formatter with I/O context and terminal.
// Most code should use the package-level functions instead (ui.Markdown, ui.Success, etc.).
func NewFormatter(ioCtx io.Context, term terminal.Terminal) Formatter {
	// Use theme-aware styles based on configured theme
	styles := theme.GetCurrentStyles()

	return &formatter{
		ioCtx:    ioCtx,
		terminal: term,
		styles:   styles,
	}
}

func (f *formatter) Styles() *theme.StyleSet {
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
		return fmt.Sprintf(iconMessageFormat, icon, text)
	}
	return style.Render(fmt.Sprintf(iconMessageFormat, icon, text))
}

// formatToast formats a toast message with icon, handling multiline messages with proper indentation.
// Splits message on newlines and indents continuation lines to align with the first line text.
//
// Parameters:
//   - icon: The icon/emoji to prefix the message (may include ANSI color codes)
//   - message: The message text (may contain newlines)
//
// Returns formatted string with newline at the end.
//
// Example:
//
//	formatToast("✓", "Done\nFile: test.txt\nSize: 1.2MB")
//	// Returns: "✓ Done\n  File: test.txt\n  Size: 1.2MB\n"
func (f *formatter) formatToast(icon, message string) string {
	lines := strings.Split(message, "\n")

	if len(lines) == 1 {
		// Single line - simple format
		return fmt.Sprintf(iconMessageFormat, icon, message) + newline
	}

	// Multiline - calculate indent for continuation lines
	// Icon + space = visual width to match
	// lipgloss.Width() handles both ANSI codes and multi-cell characters (emojis)
	iconWidth := lipgloss.Width(icon)
	indent := strings.Repeat(" ", iconWidth+1)

	// Build formatted output
	var result strings.Builder
	for i, line := range lines {
		if i == 0 {
			// First line gets the icon (potentially with color)
			result.WriteString(fmt.Sprintf(iconMessageFormat, icon, line))
		} else {
			// Continuation lines get indented
			result.WriteString(newline)
			result.WriteString(indent)
			result.WriteString(line)
		}
	}
	result.WriteString(newline)

	return result.String()
}

// Semantic formatting - uses formatToast with colored icons for multiline support.
// The styles handle color degradation automatically based on terminal capabilities.
func (f *formatter) Success(text string) string {
	icon := f.styles.Success.Render("✓")
	// Remove the trailing newline that formatToast adds since callers will add it
	result := f.formatToast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Successf(format string, a ...interface{}) string {
	return f.Success(fmt.Sprintf(format, a...))
}

func (f *formatter) Warning(text string) string {
	icon := f.styles.Warning.Render("⚠")
	// Remove the trailing newline that formatToast adds since callers will add it
	result := f.formatToast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Warningf(format string, a ...interface{}) string {
	return f.Warning(fmt.Sprintf(format, a...))
}

func (f *formatter) Error(text string) string {
	icon := f.styles.Error.Render("✗")
	// Remove the trailing newline that formatToast adds since callers will add it
	result := f.formatToast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Errorf(format string, a ...interface{}) string {
	return f.Error(fmt.Sprintf(format, a...))
}

func (f *formatter) Info(text string) string {
	icon := f.styles.Info.Render("ℹ")
	// Remove the trailing newline that formatToast adds since callers will add it
	result := f.formatToast(icon, text)
	return strings.TrimSuffix(result, newline)
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

// Markdown returns the rendered markdown string (pure function, no I/O).
// For writing markdown to channels, use package-level ui.Markdown() or ui.MarkdownMessage().
func (f *formatter) Markdown(content string) (string, error) {
	// Determine max width from config or terminal
	maxWidth := f.ioCtx.Config().AtmosConfig.Settings.Terminal.MaxWidth
	if maxWidth == 0 {
		// Use terminal width if available
		termWidth := f.terminal.Width(terminal.Stdout)
		if termWidth > 0 {
			maxWidth = termWidth
		}
	}

	// Build glamour options with theme-aware styling
	var opts []glamour.TermRendererOption

	if maxWidth > 0 {
		opts = append(opts, glamour.WithWordWrap(maxWidth))
	}

	// Use theme-aware glamour styles
	if f.terminal.ColorProfile() != terminal.ColorNone {
		themeName := f.ioCtx.Config().AtmosConfig.Settings.Terminal.Theme
		if themeName == "" {
			themeName = "default"
		}
		glamourStyle, err := theme.GetGlamourStyleForTheme(themeName)
		if err == nil {
			opts = append(opts, glamour.WithStylesFromJSONBytes(glamourStyle))
		}
		// Fallback to notty style if theme conversion fails
	} else {
		opts = append(opts, glamour.WithStylePath("notty"))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		// Degrade gracefully: return plain content if renderer creation fails
		return content, err
	}
	defer renderer.Close()

	rendered, err := renderer.Render(content)
	if err != nil {
		// Degrade gracefully: return plain content if rendering fails
		return content, err
	}

	return rendered, nil
}
