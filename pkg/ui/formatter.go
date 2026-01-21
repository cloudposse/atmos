package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Character constants.
	newline = "\n"
	tab     = "\t"
	space   = " "

	// ANSI escape sequences.
	clearLine = "\r\x1b[K" // Carriage return + clear from cursor to end of line

	// Format templates.
	iconMessageFormat = "%s %s"

	// Formatting constants.
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

	// Configure lipgloss global color profile based on terminal capabilities.
	// This ensures that all lipgloss styles (including theme.Styles) respect
	// terminal color settings like NO_COLOR.
	configureColorProfile(globalTerminal)

	// Create formatter with I/O context and terminal
	globalFormatter = NewFormatter(ioCtx, globalTerminal).(*formatter)
	Format = globalFormatter // Also expose for advanced use
}

// Reset clears all UI globals (formatter, I/O, terminal).
// This is primarily used in tests to ensure clean state between test executions.
func Reset() {
	formatterMu.Lock()
	defer formatterMu.Unlock()
	globalIO = nil
	globalFormatter = nil
	globalTerminal = nil
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

// Markdown writes rendered markdown to stdout (data channel).
// Use this for help text, documentation, and other pipeable formatted content.
// Note: Delegates to globalFormatter.Markdown() for rendering, then writes to data channel.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Markdown(content string) {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalFormatter == nil || globalIO == nil {
		log.Debug("ui.Markdown called before InitFormatter")
		return
	}

	rendered, err := globalFormatter.Markdown(content)
	if err != nil {
		// Degrade gracefully - write plain content if rendering fails
		rendered = content
	}

	if _, writeErr := fmt.Fprint(globalIO.Data(), rendered); writeErr != nil {
		log.Debug("ui.Markdown write failed", "error", writeErr)
	}
}

// Markdownf writes formatted markdown to stdout (data channel).
func Markdownf(format string, a ...interface{}) {
	content := fmt.Sprintf(format, a...)
	Markdown(content)
}

// MarkdownMessage writes rendered markdown to stderr (UI channel).
// Use this for formatted UI messages and errors.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func MarkdownMessage(content string) {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalFormatter == nil || globalIO == nil {
		log.Debug("ui.MarkdownMessage called before InitFormatter")
		return
	}

	rendered, err := globalFormatter.Markdown(content)
	if err != nil {
		// Degrade gracefully - write plain content if rendering fails
		rendered = content
	}

	if _, writeErr := fmt.Fprint(globalIO.UI(), rendered); writeErr != nil {
		log.Debug("ui.MarkdownMessage write failed", "error", writeErr)
	}
}

// MarkdownMessagef writes formatted markdown to stderr (UI channel).
func MarkdownMessagef(format string, a ...interface{}) {
	content := fmt.Sprintf(format, a...)
	MarkdownMessage(content)
}

// Success writes a success message with green checkmark to stderr (UI channel).
// Flow: ui.Success() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Success(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Success called before InitFormatter")
		return
	}
	formatted := f.Success(text) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Success write failed", "error", writeErr)
	}
}

// Successf writes a formatted success message with green checkmark to stderr (UI channel).
// Flow: ui.Successf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Successf(format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Successf called before InitFormatter")
		return
	}
	formatted := f.Successf(format, a...) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Successf write failed", "error", writeErr)
	}
}

// Error writes an error message with red X to stderr (UI channel).
// Flow: ui.Error() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Error(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Error called before InitFormatter")
		return
	}
	formatted := f.Error(text) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Error write failed", "error", writeErr)
	}
}

// Errorf writes a formatted error message with red X to stderr (UI channel).
// Flow: ui.Errorf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Errorf(format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Errorf called before InitFormatter")
		return
	}
	formatted := f.Errorf(format, a...) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Errorf write failed", "error", writeErr)
	}
}

// Warning writes a warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warning() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Warning(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Warning called before InitFormatter")
		return
	}
	formatted := f.Warning(text) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Warning write failed", "error", writeErr)
	}
}

// Warningf writes a formatted warning message with yellow warning sign to stderr (UI channel).
// Flow: ui.Warningf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Warningf(format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Warningf called before InitFormatter")
		return
	}
	formatted := f.Warningf(format, a...) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Warningf write failed", "error", writeErr)
	}
}

// Info writes an info message with cyan info icon to stderr (UI channel).
// Flow: ui.Info() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Info(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Info called before InitFormatter")
		return
	}
	formatted := f.Info(text) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Info write failed", "error", writeErr)
	}
}

// Infof writes a formatted info message with cyan info icon to stderr (UI channel).
// Flow: ui.Infof() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Infof(format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Infof called before InitFormatter")
		return
	}
	formatted := f.Infof(format, a...) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Infof write failed", "error", writeErr)
	}
}

// Toast writes a toast message with custom icon to stderr (UI channel).
// Flow: ui.Toast() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Toast(icon, message string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Toast called before InitFormatter")
		return
	}
	formatted := f.Toast(icon, message) // formatter.Toast() already includes trailing newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Toast write failed", "error", writeErr)
	}
}

// Toastf writes a formatted toast message with custom icon to stderr (UI channel).
// Flow: ui.Toastf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Toastf(icon, format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Toastf called before InitFormatter")
		return
	}
	formatted := f.Toastf(icon, format, a...) // formatter.Toastf() already includes trailing newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Toastf write failed", "error", writeErr)
	}
}

// Hint writes a hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hint() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Hint(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Hint called before InitFormatter")
		return
	}
	formatted := f.Hint(text) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Hint write failed", "error", writeErr)
	}
}

// Hintf writes a formatted hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hintf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Hintf(format string, a ...interface{}) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Hintf called before InitFormatter")
		return
	}
	formatted := f.Hintf(format, a...) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Hintf write failed", "error", writeErr)
	}
}

// Experimental writes an experimental feature notification with test tube icon to stderr (UI channel).
// This is used to notify users when they're using an experimental feature that may change.
// The notification behavior is controlled by settings.experimental in atmos.yaml (silence, disable, warn, error).
// The caller (root.go PersistentPreRun) handles the config check - this function just outputs.
// Flow: ui.Experimental() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Experimental(feature string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Experimental called before InitFormatter")
		return
	}

	formatted := f.Experimental(feature) + newline
	if writeErr := f.terminal.Write(formatted); writeErr != nil {
		log.Debug("ui.Experimental write failed", "error", writeErr)
	}
}

// Experimentalf writes a formatted experimental feature notification with test tube icon to stderr (UI channel).
// Flow: ui.Experimentalf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Experimentalf(format string, a ...interface{}) {
	Experimental(fmt.Sprintf(format, a...))
}

// Write writes plain text to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
func Write(text string) {
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.Write called before InitFormatter")
		return
	}
	if writeErr := f.terminal.Write(text); writeErr != nil {
		log.Debug("ui.Write write failed", "error", writeErr)
	}
}

// FormatSuccess returns a success message with green checkmark as a formatted string.
// Use this when you need the formatted string without writing (e.g., in bubbletea views).
func FormatSuccess(text string) string {
	f, err := getFormatter()
	if err != nil {
		// Fallback to unformatted
		return "✓ " + text
	}
	return f.Success(text)
}

// FormatSuccessf returns a formatted success message with green checkmark as a formatted string.
// Use this when you need the formatted string without writing (e.g., in bubbletea views).
func FormatSuccessf(format string, a ...interface{}) string {
	return FormatSuccess(fmt.Sprintf(format, a...))
}

// FormatError returns an error message with red X as a formatted string.
// Use this when you need the formatted string without writing (e.g., in bubbletea views).
func FormatError(text string) string {
	f, err := getFormatter()
	if err != nil {
		// Fallback to unformatted
		return "✗ " + text
	}
	return f.Error(text)
}

// FormatErrorf returns a formatted error message with red X as a formatted string.
// Use this when you need the formatted string without writing (e.g., in bubbletea views).
func FormatErrorf(format string, a ...interface{}) string {
	return FormatError(fmt.Sprintf(format, a...))
}

// Badge returns a styled badge with the given text, background color, and foreground color.
// Badges are compact labels with background styling, typically used for status indicators.
// The background and foreground should be hex colors (e.g., "#FF9800", "#000000").
// Use this when you need the formatted string without writing (e.g., in help text).
func Badge(text, background, foreground string) string {
	f, err := getFormatter()
	if err != nil {
		// Fallback to unformatted.
		return "[" + text + "]"
	}
	return f.Badge(text, background, foreground)
}

// FormatExperimentalBadge returns an "EXPERIMENTAL" badge using theme colors.
// Use this to indicate experimental features in help text or command descriptions.
func FormatExperimentalBadge() string {
	f, err := getFormatter()
	if err != nil {
		// Fallback to unformatted.
		return "[EXPERIMENTAL]"
	}
	if !f.SupportsColor() {
		return "[EXPERIMENTAL]"
	}
	return f.styles.ExperimentalBadge.Render("EXPERIMENTAL")
}

// Writef writes formatted text to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Writef() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Writef(format string, a ...interface{}) {
	Write(fmt.Sprintf(format, a...))
}

// Writeln writes text followed by a newline to stderr (UI channel) without icons or automatic styling.
// Flow: ui.Writeln() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Writeln(text string) {
	Write(text + newline)
}

// ClearLine clears the current line in the terminal and returns cursor to the beginning.
// Respects NO_COLOR and terminal capabilities - uses ANSI escape sequences only when supported.
// When colors are disabled, only writes carriage return to move cursor to start of line.
// This is useful for replacing spinner messages or other dynamic output with final status messages.
// Flow: ui.ClearLine() → ui.Write() → terminal.Write() → io.Write(UIStream) → masking → stderr.
// Write errors are logged but not returned since callers cannot meaningfully handle them.
//
// Example usage:
//
//	ui.ClearLine()
//	ui.Success("Operation completed successfully")
func ClearLine() {
	formatterMu.RLock()
	defer formatterMu.RUnlock()

	if globalTerminal == nil {
		log.Debug("ui.ClearLine called before InitFormatter")
		return
	}

	// Only use ANSI clear sequence if terminal supports colors.
	// When NO_COLOR=1 or color is disabled, just use carriage return.
	if globalTerminal.ColorProfile() != terminal.ColorNone {
		Write(clearLine) // \r\x1b[K - carriage return + clear to EOL
	} else {
		Write("\r") // Just carriage return when colors disabled
	}
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

// Badge renders a styled badge with background color, foreground color, bold text, and padding.
// Badges are compact labels used for status indicators like [EXPERIMENTAL], [BETA], etc.
func (f *formatter) Badge(text, background, foreground string) string {
	if !f.SupportsColor() {
		return "[" + text + "]"
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(background)).
		Foreground(lipgloss.Color(foreground)).
		Bold(true).
		Padding(0, 1).
		Render(text)
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
	rendered = strings.TrimPrefix(rendered, newline)         // Remove Glamour's leading newline
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

	// Split by newlines and trim trailing padding that Glamour adds.
	lines := atmosansi.TrimTrailingWhitespace(rendered, paragraphIndent, paragraphIndentWidth)

	if len(lines) == 0 {
		return styledIcon, nil
	}

	if len(lines) == 1 {
		// For single line: trim leading spaces from Glamour's paragraph indent
		// since the icon+space already provides visual separation.
		// Use ANSI-aware trimming since Glamour may wrap spaces in color codes.
		line := atmosansi.TrimLeftSpaces(lines[0])
		return fmt.Sprintf(iconMessageFormat, styledIcon, line), nil
	}

	// Multi-line: trim leading spaces from first line (goes next to icon).
	// Use ANSI-aware trimming since Glamour may wrap spaces in color codes.
	lines[0] = atmosansi.TrimLeftSpaces(lines[0])

	// Multi-line: first line with icon, rest indented to align under first line's text.
	result := fmt.Sprintf(iconMessageFormat, styledIcon, lines[0])

	// Calculate indent: icon width + 1 space from iconMessageFormat.
	// Use lipgloss.Width to handle multi-cell characters like emojis.
	iconWidth := lipgloss.Width(icon)
	indent := strings.Repeat(space, iconWidth+1) // +1 for the space in "%s %s" format.

	for i := 1; i < len(lines); i++ {
		// Glamour already added 2-space paragraph indent, replace with our calculated indent.
		// Use ANSI-aware trimming since Glamour may wrap spaces in color codes.
		line := atmosansi.TrimLeftSpaces(lines[i])
		result += newline + indent + line
	}

	return result, nil
}

// renderToastMarkdown renders markdown with a compact stylesheet for toast messages.
func (f *formatter) renderToastMarkdown(content string) (string, error) {
	// Build glamour options with compact toast stylesheet
	var opts []glamour.TermRendererOption

	// Enable word wrap for toast messages to respect terminal width.
	// Note: Glamour adds padding to fill width - we trim it with trimTrailingWhitespace().
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

func (f *formatter) Experimental(feature string) string {
	var message string
	if feature != "" {
		message = fmt.Sprintf("`%s` is an experimental feature. [Learn more](https://atmos.tools/experimental)", feature)
	} else {
		message = "Experimental feature. [Learn more](https://atmos.tools/experimental)"
	}
	result, _ := f.toastMarkdown(theme.IconExperimental, &f.styles.Muted, message)
	return result
}

func (f *formatter) Experimentalf(format string, a ...interface{}) string {
	return f.Experimental(fmt.Sprintf(format, a...))
}

func (f *formatter) Hint(text string) string {
	// Render the icon with muted style.
	var styledIcon string
	if f.SupportsColor() {
		styledIcon = f.styles.Muted.Render("💡")
	} else {
		styledIcon = "💡"
	}

	// Render the text with inline markdown and apply muted style.
	styledText := f.renderInlineMarkdownWithBase(text, &f.styles.Muted)

	return fmt.Sprintf(iconMessageFormat, styledIcon, styledText)
}

// renderInlineMarkdownWithBase renders inline markdown and applies a base style to the result.
// This is useful for rendering markdown within styled contexts like hints.
func (f *formatter) renderInlineMarkdownWithBase(text string, baseStyle *lipgloss.Style) string {
	// Render markdown using toast renderer for compact inline formatting.
	rendered, err := f.renderToastMarkdown(text)
	if err != nil {
		// Degrade gracefully: apply base style to plain text.
		if f.SupportsColor() && baseStyle != nil {
			return baseStyle.Render(text)
		}
		return text
	}

	// Clean up Glamour's extra newlines.
	rendered = strings.TrimPrefix(rendered, newline)
	rendered = strings.TrimSuffix(rendered, newline+newline)
	rendered = strings.TrimSuffix(rendered, newline)

	// Trim trailing padding and leading indent from Glamour.
	lines := atmosansi.TrimTrailingWhitespace(rendered, paragraphIndent, paragraphIndentWidth)
	if len(lines) == 0 {
		return ""
	}

	// For single line, trim leading spaces.
	// Use ANSI-aware trimming since Glamour may wrap spaces in color codes.
	if len(lines) == 1 {
		rendered = atmosansi.TrimLeftSpaces(lines[0])
	} else {
		// Multi-line: trim first line and rejoin.
		// Use ANSI-aware trimming since Glamour may wrap spaces in color codes.
		lines[0] = atmosansi.TrimLeftSpaces(lines[0])
		rendered = strings.Join(lines, newline)
	}

	// Apply base style if color is supported.
	if f.SupportsColor() && baseStyle != nil {
		return baseStyle.Render(rendered)
	}

	return rendered
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

	// Account for document left indent to prevent text overflow.
	// The glamour stylesheet adds theme.DocumentIndent spaces on the left.
	// Must match the Indent value in pkg/ui/theme/converter.go.
	const documentIndent = 2
	if maxWidth > documentIndent {
		maxWidth -= documentIndent
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

	// Remove trailing whitespace that glamour adds for padding.
	return atmosansi.TrimLinesRight(rendered), nil
}
