package theme

import (
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
)

// createLogLevelStyles creates log level styles with badge-like appearance.
func createLogLevelStyles(scheme *ColorScheme) map[log.Level]lipgloss.Style {
	levels := make(map[log.Level]lipgloss.Style)

	// Bold white text on colored backgrounds for better readability
	// Using 4-letter log level format as expected by snapshots
	levels[log.DebugLevel] = lipgloss.NewStyle().
		SetString("DEBU").
		Background(lipgloss.Color(scheme.LogDebug)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1) // 1 space padding left and right

	levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("INFO").
		Background(lipgloss.Color(scheme.LogInfo)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	levels[log.WarnLevel] = lipgloss.NewStyle().
		SetString("WARN").
		Background(lipgloss.Color(scheme.LogWarning)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("EROR").
		Background(lipgloss.Color(scheme.LogError)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FATL").
		Background(lipgloss.Color(scheme.LogError)). // Use error color for fatal
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	return levels
}

// GetLogStyles returns charm/log styles configured with the current theme colors.
// This includes background colors for log level badges and key-value pair styling.
func GetLogStyles(scheme *ColorScheme) *log.Styles {
	if scheme == nil {
		// Return default styles if no theme colors are available
		return log.DefaultStyles()
	}

	styles := &log.Styles{
		Levels: createLogLevelStyles(scheme),
		Keys:   make(map[string]lipgloss.Style),
		Values: make(map[string]lipgloss.Style),
	}

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
	styles := &log.Styles{
		Levels: make(map[log.Level]lipgloss.Style),
	}

	// Set log level strings without colors for no-color mode
	// Using 4-letter log level format as expected by snapshots
	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().SetString("DEBU")
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().SetString("INFO")
	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().SetString("WARN")
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().SetString("EROR")
	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().SetString("FATL")

	// Clear other style elements
	styles.Timestamp = lipgloss.NewStyle()
	styles.Message = lipgloss.NewStyle()
	styles.Key = lipgloss.NewStyle()
	styles.Value = lipgloss.NewStyle()
	styles.Prefix = lipgloss.NewStyle()
	styles.Separator = lipgloss.NewStyle()

	return styles
}
