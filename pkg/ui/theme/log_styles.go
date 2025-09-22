package theme

import (
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
)

// GetLogStyles returns charm/log styles configured with the current theme colors.
// This includes background colors for log level badges and key-value pair styling.
func GetLogStyles(scheme *ColorScheme) *log.Styles {
	if scheme == nil {
		// Return default styles if no theme colors are available
		return log.DefaultStyles()
	}

	styles := &log.Styles{
		Levels: make(map[log.Level]lipgloss.Style),
		Keys:   make(map[string]lipgloss.Style),
		Values: make(map[string]lipgloss.Style),
	}

	// Define log level styles with backgrounds and padding for badge-like appearance
	// Use bold white text on colored backgrounds for better readability
	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().
		Background(lipgloss.Color(scheme.LogDebug)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1) // Add horizontal padding for badge effect

	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		Background(lipgloss.Color(scheme.LogInfo)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().
		Background(lipgloss.Color(scheme.LogWarning)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		Background(lipgloss.Color(scheme.LogError)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().
		Background(lipgloss.Color(scheme.LogError)). // Use error color for fatal
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	// Configure key-value pair colors
	// Keys use the theme's muted color for subtle emphasis
	styles.Key = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Bold(false)

	// Values use the theme's primary color for visibility
	styles.Value = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.Primary))

	// Set timestamp style using muted color
	styles.Timestamp = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Faint(true)

	// Set message style
	styles.Message = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextPrimary))

	// Set prefix and separator styles
	styles.Prefix = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Bold(true)

	styles.Separator = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Faint(true)

	return styles
}

// GetLogStylesNoColor returns charm/log styles with no colors for --no-color mode.
func GetLogStylesNoColor() *log.Styles {
	stylesDefault := log.DefaultStyles()
	styles := &log.Styles{
		Levels: make(map[log.Level]lipgloss.Style),
	}

	// Clear all colors and formatting for no-color mode
	for level := range stylesDefault.Levels {
		styles.Levels[level] = lipgloss.NewStyle()
	}

	// Clear other style elements
	styles.Timestamp = lipgloss.NewStyle()
	styles.Message = lipgloss.NewStyle()
	styles.Key = lipgloss.NewStyle()
	styles.Value = lipgloss.NewStyle()
	styles.Prefix = lipgloss.NewStyle()
	styles.Separator = lipgloss.NewStyle()

	return styles
}
