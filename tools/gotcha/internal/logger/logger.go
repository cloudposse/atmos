package logger

import (
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	// globalLogger is the shared logger instance.
	globalLogger *log.Logger
	once         sync.Once
	// logBuffer is used to buffer log output during TUI sessions.
	logBuffer *BufferedWriter
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

// EnableBuffering redirects logger output to a buffer instead of stderr.
// This is useful during TUI sessions to prevent interference with the UI.
func EnableBuffering() {
	if logBuffer == nil {
		logBuffer = NewBufferedWriter(os.Stderr)
	}
	if globalLogger != nil {
		globalLogger.SetOutput(logBuffer)
	}
}

// DisableBuffering restores logger output to stderr and flushes any buffered content.
func DisableBuffering() {
	if globalLogger != nil {
		globalLogger.SetOutput(os.Stderr)
	}
	if logBuffer != nil {
		_ = logBuffer.Flush()
		logBuffer = nil
	}
}

// FlushBuffer writes any buffered log output to stderr.
func FlushBuffer() {
	if logBuffer != nil {
		_ = logBuffer.Flush()
	}
}

// NewStyledLogger creates a new logger with the standard gotcha styling.
func NewStyledLogger() *log.Logger {
	output := io.Writer(os.Stderr)
	if logBuffer != nil {
		output = logBuffer
	}
	logger := log.New(output)
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