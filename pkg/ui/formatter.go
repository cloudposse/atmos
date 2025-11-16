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
	space   = " "

	// Formatting constants.
	iconMessageFormat    = "%s %s"
	paragraphIndent      = "  " // 2-space indent for paragraph continuation
	paragraphIndentWidth = 2    // Width of paragraph indent
)

var (
	// Global formatter instance and I/O context.
	globalIO        io.Context
	globalFormatter *formatter
	globalTerminal  terminal.Terminal
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
	globalTerminal = terminal.New(terminal.WithIO(termWriter))

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
// Flow: ui.Success() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Success(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Success(text) + newline
	return f.terminal.Write(formatted)
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
// Flow: ui.Successf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Successf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Successf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Error writes an error message with red X to stderr (UI channel).
// Flow: ui.Error() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Error(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Error(text) + newline
	return f.terminal.Write(formatted)
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
// Flow: ui.Errorf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Errorf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Errorf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warning() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warning(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Warning(text) + newline
	return f.terminal.Write(formatted)
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warningf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warningf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Warningf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Info writes an info message with cyan info icon to stderr (UI channel).
// Flow: ui.Info() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Info(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Info(text) + newline
	return f.terminal.Write(formatted)
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
// Flow: ui.Infof() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Infof(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Infof(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Toast writes a toast message with custom icon to stderr (UI channel).
// Flow: ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Toast(icon, message string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Toast(icon, message) // formatter.Toast() already includes trailing newline
	return f.terminal.Write(formatted)
}

// Toastf writes a formatted toast message with custom icon to stderr (UI channel).
// Flow: ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Toastf(icon, format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Toastf(icon, format, a...) // formatter.Toastf() already includes trailing newline
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
// Returns formatted string: "{colored icon} {text}" where only the icon is colored.
func (f *formatter) StatusMessage(icon string, style *lipgloss.Style, text string) string {
	if !f.SupportsColor() {
		return fmt.Sprintf(iconMessageFormat, icon, text)
	}
	// Style only the icon, not the entire message.
	styledIcon := style.Render(icon)
	return fmt.Sprintf(iconMessageFormat, styledIcon, text)
}

// Toast renders markdown text with an icon prefix and auto-indents multi-line content.
// Returns the formatted string with a trailing newline.
func (f *formatter) Toast(icon, message string) string {
	result, _ := f.toastMarkdown(icon, nil, message)
	return result + newline
}

// Toastf renders formatted markdown text with an icon prefix.
func (f *formatter) Toastf(icon, format string, a ...interface{}) string {
	return f.Toast(icon, fmt.Sprintf(format, a...))
}

// trimTrailingWhitespace splits rendered markdown by newlines and trims trailing spaces
// that Glamour adds for padding. For empty lines (all whitespace), it preserves
// the leading indent (first 2 spaces) to maintain paragraph structure.
func trimTrailingWhitespace(rendered string) []string {
	lines := strings.Split(rendered, newline)
	for i := range lines {
		trimmed := strings.TrimRight(lines[i], space)
		// If line became empty after trimming but had content before,
		// it was an empty line with indent - preserve the indent
		if trimmed == "" && len(lines[i]) > 0 {
			// Preserve up to 2 leading spaces for paragraph indent
			if len(lines[i]) >= paragraphIndentWidth {
				lines[i] = paragraphIndent
			} else {
				lines[i] = lines[i][:len(lines[i])] // Keep whatever spaces there were
			}
		} else {
			lines[i] = trimmed
		}
	}
	return lines
}

// toastMarkdown renders markdown text with preserved newlines, an icon prefix, and auto-indents multi-line content.
// Uses a compact stylesheet for toast-style inline formatting.
func (f *formatter) toastMarkdown(icon string, style *lipgloss.Style, text string) (string, error) {
	// Render markdown with toast-specific compact stylesheet
	rendered, err := f.renderToastMarkdown(text)
	if err != nil {
		return "", err
	}

	// Glamour adds 1 leading newline and 2 trailing newlines to every output.
	// Remove these, but preserve any newlines that were in the original message.
	rendered = strings.TrimPrefix(rendered, newline)          // Remove Glamour's leading newline
	rendered = strings.TrimSuffix(rendered, newline+newline) // Remove Glamour's trailing newlines
	// If there's still a trailing newline, it was from the original message.
	if !strings.HasSuffix(rendered, newline) && strings.HasSuffix(text, newline) {
		// Original had trailing newline but rendering lost it, add it back.
		rendered += newline
	}

	// Style the icon if color is supported
	var styledIcon string
	if f.SupportsColor() && style != nil {
		styledIcon = style.Render(icon)
	} else {
		styledIcon = icon
	}

	// Split by newlines and trim trailing padding that Glamour adds
	lines := trimTrailingWhitespace(rendered)

	if len(lines) == 0 {
		return styledIcon, nil
	}

	if len(lines) == 1 {
		// For single line: trim leading spaces from Glamour's paragraph indent
		// since the icon+space already provides visual separation
		line := strings.TrimLeft(lines[0], space)
		return fmt.Sprintf(iconMessageFormat, styledIcon, line), nil
	}

	// Multi-line: trim leading spaces from first line (goes next to icon)
	lines[0] = strings.TrimLeft(lines[0], space)

	// Multi-line: first line with icon, rest indented to align under first line's text
	result := fmt.Sprintf(iconMessageFormat, styledIcon, lines[0])

	// Calculate indent: icon width + 1 space from iconMessageFormat
	// Use lipgloss.Width to handle multi-cell characters like emojis
	iconWidth := lipgloss.Width(icon)
	indent := strings.Repeat(space, iconWidth+1) // +1 for the space in "%s %s" format

	for i := 1; i < len(lines); i++ {
		// Glamour already added 2-space paragraph indent, replace with our calculated indent
		line := strings.TrimLeft(lines[i], space) // Remove Glamour's indent
		result += newline + indent + line
	}

	return result, nil
}

// renderToastMarkdown renders markdown with a compact stylesheet for toast messages.
func (f *formatter) renderToastMarkdown(content string) (string, error) {
	// Build glamour options with compact toast stylesheet
	var opts []glamour.TermRendererOption

	// Enable word wrap for toast messages to respect terminal width
	// Note: Glamour adds padding to fill width - we trim it later
	maxWidth := f.ioCtx.Config().AtmosConfig.Settings.Terminal.MaxWidth
	if maxWidth == 0 {
		// Use terminal width if available
		termWidth := f.terminal.Width(terminal.Stdout)
		if termWidth > 0 {
			maxWidth = termWidth
		}
	}
	if maxWidth > 0 {
		opts = append(opts, glamour.WithWordWrap(maxWidth))
	}
	opts = append(opts, glamour.WithPreservedNewLines())

	// Get theme-based glamour style and modify it for compact toast rendering
	if f.terminal.ColorProfile() != terminal.ColorNone {
		themeName := f.ioCtx.Config().AtmosConfig.Settings.Terminal.Theme
		if themeName == "" {
			themeName = "default"
		}
		glamourStyle, err := theme.GetGlamourStyleForTheme(themeName)
		if err == nil {
			// Modify the theme style to have zero margins
			// Parse the existing theme and override margin settings
			opts = append(opts, glamour.WithStylesFromJSONBytes(glamourStyle))
		}
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

// Semantic formatting - all use toastMarkdown for markdown rendering and icon styling.
func (f *formatter) Success(text string) string {
	result, _ := f.toastMarkdown("✓", &f.styles.Success, text)
	return result
}

func (f *formatter) Successf(format string, a ...interface{}) string {
	return f.Success(fmt.Sprintf(format, a...))
}

func (f *formatter) Warning(text string) string {
	result, _ := f.toastMarkdown("⚠", &f.styles.Warning, text)
	return result
}

func (f *formatter) Warningf(format string, a ...interface{}) string {
	return f.Warning(fmt.Sprintf(format, a...))
}

func (f *formatter) Error(text string) string {
	result, _ := f.toastMarkdown("✗", &f.styles.Error, text)
	return result
}

func (f *formatter) Errorf(format string, a ...interface{}) string {
	return f.Error(fmt.Sprintf(format, a...))
}

func (f *formatter) Info(text string) string {
	result, _ := f.toastMarkdown("ℹ", &f.styles.Info, text)
	return result
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
	return f.renderMarkdown(content, false)
}

// renderMarkdown is the internal markdown rendering implementation.
func (f *formatter) renderMarkdown(content string, preserveNewlines bool) (string, error) {
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

	// Preserve newlines if requested
	if preserveNewlines {
		opts = append(opts, glamour.WithPreservedNewLines())
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
