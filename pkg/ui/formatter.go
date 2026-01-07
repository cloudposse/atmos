package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// Hint writes a hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hint() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Hint(text string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Hint(text) + newline
	return f.terminal.Write(formatted)
}

// Hintf writes a formatted hint/tip message with lightbulb icon to stderr (UI channel).
// This is a convenience wrapper with themed hint icon and muted color.
// Flow: ui.Hintf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Hintf(format string, a ...interface{}) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}
	formatted := f.Hintf(format, a...) + newline
	return f.terminal.Write(formatted)
}

// Experimental writes an experimental feature notification with test tube icon to stderr (UI channel).
// This is used to notify users when they're using an experimental feature that may change.
// The notification can be disabled via settings.terminal.experimental_warnings: false in atmos.yaml.
// Flow: ui.Experimental() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Experimental(feature string) error {
	f, err := getFormatter()
	if err != nil {
		return err
	}

	// Check if experimental warnings are disabled.
	if f.ioCtx != nil && f.ioCtx.Config() != nil {
		if !f.ioCtx.Config().AtmosConfig.Settings.Terminal.ExperimentalWarnings {
			return nil
		}
	}

	formatted := f.Experimental(feature) + newline
	return f.terminal.Write(formatted)
}

// Experimentalf writes a formatted experimental feature notification with test tube icon to stderr (UI channel).
// Flow: ui.Experimentalf() → terminal.Write() → io.Write(UIStream) → masking → stderr.
func Experimentalf(format string, a ...interface{}) error {
	return Experimental(fmt.Sprintf(format, a...))
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

// isANSIStart checks if position i marks the start of an ANSI escape sequence.
func isANSIStart(s string, i int) bool {
	return s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '['
}

// skipANSISequence advances past an ANSI escape sequence starting at position i.
// Returns the index after the sequence terminator.
func skipANSISequence(s string, i int) int {
	i += 2 // Skip ESC and [.
	for i < len(s) && !isANSITerminator(s[i]) {
		i++
	}
	if i < len(s) {
		i++ // Skip terminator.
	}
	return i
}

// isANSITerminator checks if byte b is an ANSI sequence terminator (A-Z or a-z).
func isANSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// copyContentAndANSI copies characters and ANSI codes from s until plainIdx reaches targetLen.
// Returns the result builder pointer and the final position in s.
func copyContentAndANSI(s string, targetLen int) (*strings.Builder, int) {
	result := &strings.Builder{}
	plainIdx := 0
	i := 0

	for i < len(s) && plainIdx < targetLen {
		if isANSIStart(s, i) {
			start := i
			i = skipANSISequence(s, i)
			result.WriteString(s[start:i])
		} else {
			result.WriteByte(s[i])
			plainIdx++
			i++
		}
	}

	return result, i
}

// trimRightSpaces removes only trailing spaces (not tabs) from an ANSI-coded string while
// preserving all ANSI escape sequences on the actual content.
// This is useful for removing Glamour's padding spaces while preserving intentional tabs.
func trimRightSpaces(s string) string {
	stripped := ansi.Strip(s)
	trimmed := strings.TrimRight(stripped, " ")

	if trimmed == stripped {
		return s
	}
	if trimmed == "" {
		return ""
	}

	result, i := copyContentAndANSI(s, len(trimmed))

	// Capture any trailing ANSI codes that immediately follow the last character.
	for i < len(s) && isANSIStart(s, i) {
		start := i
		i = skipANSISequence(s, i)
		result.WriteString(s[start:i])
	}

	return result.String()
}

// isWhitespace checks if byte b is a space or tab.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t'
}

// processTrailingANSICodes processes ANSI codes after content, preserving reset codes
// but not color codes that wrap trailing whitespace.
func processTrailingANSICodes(s string, i int, result *strings.Builder) {
	for i < len(s) && isANSIStart(s, i) {
		start := i
		i = skipANSISequence(s, i)

		// Check what comes after this ANSI code.
		if shouldIncludeTrailingANSI(s, i, start, result) {
			return
		}
	}
}

// shouldIncludeTrailingANSI determines whether to include a trailing ANSI code and stop processing.
// Returns true if processing should stop.
func shouldIncludeTrailingANSI(s string, i, start int, result *strings.Builder) bool {
	// Whitespace or end of string directly after this code - include and stop.
	if i >= len(s) || isWhitespace(s[i]) {
		result.WriteString(s[start:i])
		return true
	}

	// Another ANSI code follows - peek ahead.
	if isANSIStart(s, i) {
		nextEnd := skipANSISequence(s, i)
		if nextEnd < len(s) && isWhitespace(s[nextEnd]) {
			// Next code wraps whitespace - include current and stop.
			result.WriteString(s[start:i])
			return true
		}
		// Next code doesn't wrap whitespace - include and continue.
		result.WriteString(s[start:i])
		return false
	}

	// Other content follows - include the code.
	result.WriteString(s[start:i])
	return false
}

// TrimRight removes trailing whitespace from an ANSI-coded string while
// preserving all ANSI escape sequences on the actual content.
// This is useful for removing Glamour's padding spaces that are wrapped in ANSI codes.
func TrimRight(s string) string {
	stripped := ansi.Strip(s)
	trimmed := strings.TrimRight(stripped, " \t")

	if trimmed == stripped {
		return s
	}
	if trimmed == "" {
		return ""
	}

	result, i := copyContentAndANSI(s, len(trimmed))
	processTrailingANSICodes(s, i, result)

	return result.String()
}

// TrimLinesRight trims trailing whitespace from each line in a multi-line string.
// This is useful after lipgloss.Render() which pads all lines to the same width.
// Uses ANSI-aware TrimRight to handle whitespace wrapped in ANSI codes.
func TrimLinesRight(s string) string {
	lines := strings.Split(s, newline)
	for i, line := range lines {
		lines[i] = TrimRight(line)
	}
	return strings.Join(lines, newline)
}

// trimTrailingWhitespace splits rendered markdown by newlines and trims trailing spaces
// that Glamour adds for padding (including ANSI-wrapped spaces). For empty lines (all whitespace),
// it preserves the leading indent (first 2 spaces) to maintain paragraph structure.
func trimTrailingWhitespace(rendered string) []string {
	lines := strings.Split(rendered, newline)
	for i := range lines {
		// Use trimRightSpaces to remove trailing spaces while preserving tabs
		line := trimRightSpaces(lines[i])

		// If line became empty after trimming but had content before,
		// it was an empty line with indent - preserve the indent
		if line == "" && len(lines[i]) > 0 {
			// Preserve up to 2 leading spaces for paragraph indent
			if len(lines[i]) >= paragraphIndentWidth {
				lines[i] = paragraphIndent
			} else {
				lines[i] = lines[i][:len(lines[i])] // Keep whatever spaces there were
			}
		} else {
			lines[i] = line
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
	message := fmt.Sprintf("**%s** is an experimental feature. [Learn more](https://atmos.tools/experimental)", feature)
	result, _ := f.toastMarkdown(theme.IconExperimental, &f.styles.Experimental, message)
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
	lines := trimTrailingWhitespace(rendered)
	if len(lines) == 0 {
		return ""
	}

	// For single line, trim leading spaces.
	if len(lines) == 1 {
		rendered = strings.TrimLeft(lines[0], space)
	} else {
		// Multi-line: trim first line and rejoin.
		lines[0] = strings.TrimLeft(lines[0], space)
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
	return TrimLinesRight(rendered), nil
}
