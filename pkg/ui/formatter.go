package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Character constants.
	newline = "\n"
	tab     = "\t"

	// ANSI escape sequences.
	clearLine = "\r\x1b[K" // Carriage return + clear from cursor to end of line

	// Format templates.
	iconMessageFormat = "%s %s"
)

var (
	// Global instances for I/O and formatting.
	globalIO        io.Context
	globalTerminal  terminal.Terminal
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
	globalTerminal = terminal.New(terminal.WithIO(termWriter))

	// Configure lipgloss global color profile based on terminal capabilities.
	// This ensures that all lipgloss styles (including theme.Styles) respect
	// terminal color settings like NO_COLOR.
	configureColorProfile(globalTerminal)

	// Create formatter with I/O context and terminal
	// Note: Formatter still gets terminal for terminal capability detection (color profile, width, etc.)
	globalFormatter = NewFormatter(ioCtx, globalTerminal).(*formatter)
	Format = globalFormatter // Also expose for advanced use
}

// Reset clears the global formatter and I/O context.
// This is primarily used in tests to ensure clean state between test executions.
func Reset() {
	formatterMu.Lock()
	defer formatterMu.Unlock()
	globalIO = nil
	globalFormatter = nil
	Format = nil
}

// configureColorProfile sets the global lipgloss color profile based on terminal capabilities.
// This ensures all lipgloss styles respect NO_COLOR and other terminal color settings.
func configureColorProfile(term terminal.Terminal) {
	profile := term.ColorProfile()

	// Map terminal color profile to termenv profile for lipgloss
	var termProfile termenv.Profile
	switch profile {
	case terminal.ColorNone:
		termProfile = termenv.Ascii
	case terminal.Color16:
		termProfile = termenv.ANSI
	case terminal.Color256:
		termProfile = termenv.ANSI256
	case terminal.ColorTrue:
		termProfile = termenv.TrueColor
	default:
		termProfile = termenv.Ascii
	}

	setColorProfileInternal(termProfile)
}

// setColorProfileInternal applies a color profile to all color-dependent systems.
// This centralizes the color profile configuration for lipgloss, theme, and logger.
func setColorProfileInternal(profile termenv.Profile) {
	// Set the global lipgloss color profile
	lipgloss.SetColorProfile(profile)

	// Force theme styles to be regenerated with the new color profile.
	// This is critical because theme.CurrentStyles caches lipgloss styles that
	// bake in ANSI codes at creation time. When the color profile changes,
	// we must regenerate the styles.
	theme.InvalidateStyleCache()

	// Reinitialize logger to respect the new color profile.
	// The logger is initialized in init() with a default color profile,
	// so we need to explicitly reconfigure it when the color profile changes.
	log.Default().SetColorProfile(profile)
}

// SetColorProfile sets the color profile for all UI systems (lipgloss, theme, logger).
// This is primarily intended for testing when environment variables are set after
// package initialization. For normal operation, color profiles are automatically
// configured during InitFormatter() based on terminal capabilities.
//
// Example usage in tests:
//
//	t.Setenv("NO_COLOR", "1")
//	ui.SetColorProfile(termenv.Ascii)
func SetColorProfile(profile termenv.Profile) {
	setColorProfileInternal(profile)
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
// Flow: ui.Toast() → ui.Format.Toast() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
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
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Toast(icon, message)
	return Write(formatted)
}

// Toastf writes a formatted toast notification with a custom icon to stderr (UI channel).
// This is the primary pattern for formatted toast-style notifications with flexible icon support.
// Flow: ui.Toastf() → ui.Format.Toastf() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
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
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Toastf(icon, format, a...)
	return Write(formatted)
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
// This is a convenience wrapper with themed success icon and color.
// Flow: ui.Success() → ui.Format.Success() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Success(text string) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Success(text) + newline
	return Write(formatted)
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
// This is a convenience wrapper with themed success icon and color.
// Flow: ui.Successf() → ui.Format.Successf() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Successf(format string, a ...interface{}) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Successf(format, a...) + newline
	return Write(formatted)
}

// Error writes an error message with red X to stderr (UI channel).
// This is a convenience wrapper with themed error icon and color.
// Flow: ui.Error() → ui.Format.Error() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Error(text string) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Error(text) + newline
	return Write(formatted)
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
// This is a convenience wrapper with themed error icon and color.
// Flow: ui.Errorf() → ui.Format.Errorf() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Errorf(format string, a ...interface{}) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Errorf(format, a...) + newline
	return Write(formatted)
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
// This is a convenience wrapper with themed warning icon and color.
// Flow: ui.Warning() → ui.Format.Warning() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warning(text string) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Warning(text) + newline
	return Write(formatted)
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
// This is a convenience wrapper with themed warning icon and color.
// Flow: ui.Warningf() → ui.Format.Warningf() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Warningf(format string, a ...interface{}) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Warningf(format, a...) + newline
	return Write(formatted)
}

// Info writes an info message with cyan info icon to stderr (UI channel).
// This is a convenience wrapper with themed info icon and color.
// Flow: ui.Info() → ui.Format.Info() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Info(text string) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Info(text) + newline
	return Write(formatted)
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
// This is a convenience wrapper with themed info icon and color.
// Flow: ui.Infof() → ui.Format.Infof() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Infof(format string, a ...interface{}) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Infof(format, a...) + newline
	return Write(formatted)
}

// Hint writes a hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hint() → ui.Format.Hint() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Hint(text string) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Hint(text) + newline
	return Write(formatted)
}

// Hintf writes a formatted hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hintf() → ui.Format.Hintf() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Hintf(format string, a ...interface{}) error {
	if Format == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}
	formatted := Format.Hintf(format, a...) + newline
	return Write(formatted)
}

// Write writes plain text to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Write(text string) error {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalTerminal == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}

	return globalTerminal.Write(text)
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

// ClearLine clears the current line in the terminal and returns cursor to the beginning.
// Respects NO_COLOR and terminal capabilities - uses ANSI escape sequences only when supported.
// When colors are disabled, only writes carriage return to move cursor to start of line.
// This is useful for replacing spinner messages or other dynamic output with final status messages.
// Flow: ui.ClearLine() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
//
// Example usage:
//
//	// Clear spinner line and show success message
//	_ = ui.ClearLine()
//	_ = ui.Success("Operation completed successfully")
func ClearLine() error {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalTerminal == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}

	// Only use ANSI clear sequence if terminal supports colors.
	// When NO_COLOR=1 or color is disabled, just use carriage return.
	if globalTerminal.ColorProfile() != terminal.ColorNone {
		return Write(clearLine) // \r\x1b[K - carriage return + clear to EOL
	}
	return Write("\r") // Just carriage return when colors disabled
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

// Toast formats a toast message with icon, handling multiline messages with proper indentation.
// Splits message on newlines and indents continuation lines to align with the first line text.
// This is a pure formatting function - returns a string, does no I/O.
//
// Parameters:
//   - icon: The icon/emoji to prefix the message (may include ANSI color codes)
//   - message: The message text (may contain newlines)
//
// Returns formatted string with newline at the end.
//
// Example:
//
//	Toast("✓", "Done\nFile: test.txt\nSize: 1.2MB")
//	// Returns: "✓ Done\n  File: test.txt\n  Size: 1.2MB\n"
func (f *formatter) Toast(icon, message string) string {
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

// Toastf formats a toast message with printf-style formatting.
// This is a pure formatting function - returns a string, does no I/O.
func (f *formatter) Toastf(icon, format string, a ...interface{}) string {
	message := fmt.Sprintf(format, a...)
	return f.Toast(icon, message)
}

// Semantic formatting - uses Toast with colored icons for multiline support.
// The styles handle color degradation automatically based on terminal capabilities.
func (f *formatter) Success(text string) string {
	icon := f.styles.Success.Render("✓")
	// Remove the trailing newline that Toast adds since callers will add it
	result := f.Toast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Successf(format string, a ...interface{}) string {
	return f.Success(fmt.Sprintf(format, a...))
}

func (f *formatter) Warning(text string) string {
	icon := f.styles.Warning.Render("⚠")
	// Remove the trailing newline that Toast adds since callers will add it
	result := f.Toast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Warningf(format string, a ...interface{}) string {
	return f.Warning(fmt.Sprintf(format, a...))
}

func (f *formatter) Error(text string) string {
	icon := f.styles.Error.Render("✗")
	// Remove the trailing newline that Toast adds since callers will add it
	result := f.Toast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Errorf(format string, a ...interface{}) string {
	return f.Error(fmt.Sprintf(format, a...))
}

func (f *formatter) Info(text string) string {
	icon := f.styles.Info.Render("ℹ")
	// Remove the trailing newline that Toast adds since callers will add it
	result := f.Toast(icon, text)
	return strings.TrimSuffix(result, newline)
}

func (f *formatter) Infof(format string, a ...interface{}) string {
	return f.Info(fmt.Sprintf(format, a...))
}

func (f *formatter) Hint(text string) string {
	return f.StatusMessage("💡", &f.styles.Muted, text)
}

func (f *formatter) Hintf(format string, a ...interface{}) string {
	return f.Hint(fmt.Sprintf(format, a...))
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

	// Remove trailing whitespace that glamour adds for padding.
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	return strings.Join(lines, "\n"), nil
}

// renderInlineMarkdownWithBase renders inline markdown while preserving a base style.
// The base style is reapplied after each inline fragment to maintain consistent coloring.
// This solves the issue where lipgloss reset codes cancel outer styling.
func (f *formatter) renderInlineMarkdownWithBase(text string, baseStyle *lipgloss.Style) string {
	// If no color support, strip markdown and return plain text.
	if !f.SupportsColor() {
		// Strip **bold**.
		text = strings.ReplaceAll(text, "**", "")
		// Strip `code`.
		text = strings.ReplaceAll(text, "`", "")
		return text
	}

	// Process **bold** markers.
	boldStyle := lipgloss.NewStyle().Bold(true)
	if baseStyle != nil {
		// Inherit base style and add bold.
		boldStyle = baseStyle.Bold(true)
	}
	result := text

	// Replace **text** with bold styling.
	for {
		start := strings.Index(result, "**")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2

		// Extract the text between markers.
		boldText := result[start+2 : end]
		styledText := boldStyle.Render(boldText)

		// Replace in result.
		result = result[:start] + styledText + result[end+2:]
	}

	// Process `code` markers.
	codeStyle := f.styles.Help.Code
	if baseStyle != nil {
		// Inherit base style properties and overlay code style.
		codeStyle = baseStyle.Inherit(f.styles.Help.Code)
	}
	for {
		start := strings.Index(result, "`")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1

		// Extract the text between markers.
		codeText := result[start+1 : end]
		styledText := codeStyle.Render(codeText)

		// Replace in result.
		result = result[:start] + styledText + result[end+1:]
	}

	return result
}
