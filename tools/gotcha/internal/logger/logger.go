package logger

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
)

var (
	// GlobalLogger is the shared logger instance.
	globalLogger *log.Logger
	once         sync.Once
)

// GetLogger returns the configured global logger instance.
// If the logger hasn't been configured yet, it returns a default styled logger.
func GetLogger() *log.Logger {
	once.Do(func() {
		if globalLogger == nil {
			// Create a default logger with consistent styling if not already configured
			globalLogger = NewStyledLogger()
		}
	})
	return globalLogger
}

// SetLogger sets the global logger instance.
func SetLogger(logger *log.Logger) {
	globalLogger = logger
}

// NewStyledLogger creates a new logger with the standard gotcha styling.
func NewStyledLogger() *log.Logger {
	logger := log.New(os.Stderr)
	logger.SetStyles(&log.Styles{
		Levels: map[log.Level]lipgloss.Style{
			log.DebugLevel: lipgloss.NewStyle().
				SetString("DEBUG").
				Background(lipgloss.Color("#3F51B5")). // Indigo background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.InfoLevel: lipgloss.NewStyle().
				SetString("INFO").
				Background(lipgloss.Color("#4CAF50")). // Green background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.WarnLevel: lipgloss.NewStyle().
				SetString("WARN").
				Background(lipgloss.Color("#FF9800")). // Orange background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.ErrorLevel: lipgloss.NewStyle().
				SetString("ERROR").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.FatalLevel: lipgloss.NewStyle().
				SetString("FATAL").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#FFFFFF")). // White foreground
				Padding(0, 1),
		},
		// Style the keys with a darker gray color
		Key: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")). // Dark gray for keys
			Bold(true),
		// Values stay with their default styling (no change)
		Value: lipgloss.NewStyle(),
		// Optional: style the separator between key and value
		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#999999")), // Medium gray for separator
	})
	return logger
}
