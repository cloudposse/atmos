//go:build !linting
// +build !linting

package main

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// Define TRACE log level (one step below DEBUG).
const TraceLevel log.Level = log.DebugLevel - 1

// CustomLogger wraps log.Logger to add a Trace method.
type CustomLogger struct {
	*log.Logger
}

// Trace logs a message at TRACE level.
func (l *CustomLogger) Trace(msg string, keyvals ...any) {
	l.Log(TraceLevel, msg, keyvals...)
}

func main() {
	// Initialize structured logger
	baseLogger := log.NewWithOptions(os.Stdout, log.Options{
		Level: TraceLevel, // Change this to control log filtering
	})

	// Wrap base logger in CustomLogger
	logger := &CustomLogger{Logger: baseLogger}

	// Ensure Lipgloss uses terminal colors
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Define styles for different log levels
	styles := log.DefaultStyles()

	// Custom style for TRACE level
	styles.Levels[TraceLevel] = lipgloss.NewStyle().
		SetString("TRCE").
		Padding(0, 0).
		Background(lipgloss.Color("242")). // Gray background
		Foreground(lipgloss.Color("15"))   // White text

	// Apply styles
	logger.SetStyles(styles)
	logger.SetColorProfile(termenv.TrueColor)

	// Log messages with different levels
	logger.Trace("Tracing execution flow...", "function", "main", "step", 1)
	logger.Debug("Debugging an issue...", "module", "auth")
	logger.Info("Starting Atmos CLI...", "version", "1.0.0")
	logger.Warn("Potential misconfiguration detected", "config", "atmos.yaml")
	logger.Error("Fatal error occurred", "reason", "connection timeout")
	// Hint: Change logger.Level to log.WarnLevel and rerun to see filtering in action.
}
