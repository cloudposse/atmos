package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

// Global logger instance
var Logger *log.Logger

// Define checkmark styles for use across the application
var (
	checkMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D700")).SetString("✓")
	xMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).SetString("x")
)

// InitLogger initializes the global logger instance
func InitLogger() {
	// Initialize logger with custom options
	Logger = log.NewWithOptions(os.Stderr, log.Options{
		Level:           log.WarnLevel, // Default to warning level
		ReportCaller:    false,
		ReportTimestamp: true,
		TimeFormat:      "15:04:05",
	})

	// Set custom styles for better visibility
	styles := log.DefaultStyles()
	styles.Levels[log.DebugLevel] = styles.Levels[log.DebugLevel].Foreground(lipgloss.Color("240"))
	styles.Levels[log.InfoLevel] = styles.Levels[log.InfoLevel].Foreground(lipgloss.Color("32"))
	styles.Levels[log.WarnLevel] = styles.Levels[log.WarnLevel].Foreground(lipgloss.Color("33"))
	styles.Levels[log.ErrorLevel] = styles.Levels[log.ErrorLevel].Foreground(lipgloss.Color("31"))
	Logger.SetStyles(styles)
}

// SetLogLevel sets the log level based on the provided string
func SetLogLevel(level string) error {
	switch level {
	case "debug":
		Logger.SetLevel(log.DebugLevel)
	case "info":
		Logger.SetLevel(log.InfoLevel)
	case "warn", "warning":
		Logger.SetLevel(log.WarnLevel)
	case "error":
		Logger.SetLevel(log.ErrorLevel)
	default:
		return fmt.Errorf("invalid log level: %s. Valid levels are: debug, info, warn, error", level)
	}
	return nil
}

// SuppressLogger temporarily sets the log level to FatalLevel to suppress all output
func SuppressLogger() func() {
	// Save current level
	originalLevel := Logger.GetLevel()

	// Set to FatalLevel to suppress all output
	Logger.SetLevel(log.FatalLevel)

	// Return function to restore original level
	return func() {
		Logger.SetLevel(originalLevel)
	}
}
