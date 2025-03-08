package logger

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var helperLogger *log.Logger

func init() {
	helperLogger = GetCharmLogger()
}

// GetCharmLogger returns a pre-configured Charmbracelet logger with Atmos styling.
func GetCharmLogger() *log.Logger {
	styles := getAtmosLogStyles()
	logger := log.New(os.Stderr)
	logger.SetStyles(styles)
	return logger
}

// GetCharmLoggerWithOutput returns a pre-configured Charmbracelet logger with custom output.
func GetCharmLoggerWithOutput(output *os.File) *log.Logger {
	styles := getAtmosLogStyles()
	logger := log.New(output)
	logger.SetStyles(styles)
	return logger
}

// getAtmosLogStyles returns custom styles for the Charmbracelet logger using Atmos theme colors.
func getAtmosLogStyles() *log.Styles {
	styles := log.DefaultStyles()

	const (
		paddingVertical   = 0
		paddingHorizontal = 1
	)

	configureLogLevelStyles(styles, paddingVertical, paddingHorizontal)
	configureKeyStyles(styles)

	return styles
}

// configureLogLevelStyles configures the styles for different log levels.
func configureLogLevelStyles(styles *log.Styles, paddingVertical, paddingHorizontal int) {
	const (
		errorLevelLabel = "ERROR"
		warnLevelLabel  = "WARN"
		infoLevelLabel  = "INFO"
		debugLevelLabel = "DEBUG"
	)

	// Error.
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString(errorLevelLabel).
		Padding(paddingVertical, paddingHorizontal, paddingVertical, paddingHorizontal).
		Background(lipgloss.Color(theme.ColorPink)).
		Foreground(lipgloss.Color(theme.ColorWhite))

	// Warning.
	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().
		SetString(warnLevelLabel).
		Padding(paddingVertical, paddingHorizontal, paddingVertical, paddingHorizontal).
		Background(lipgloss.Color(theme.ColorPink)).
		Foreground(lipgloss.Color(theme.ColorDarkGray))

	// Info.
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString(infoLevelLabel).
		Padding(paddingVertical, paddingHorizontal, paddingVertical, paddingHorizontal).
		Background(lipgloss.Color(theme.ColorCyan)).
		Foreground(lipgloss.Color(theme.ColorDarkGray))

	// Debug.
	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().
		SetString(debugLevelLabel).
		Padding(paddingVertical, paddingHorizontal, paddingVertical, paddingHorizontal).
		Background(lipgloss.Color(theme.ColorBlue)).
		Foreground(lipgloss.Color(theme.ColorWhite))
}

// configureKeyStyles configures the styles for different log keys.
func configureKeyStyles(styles *log.Styles) {
	const (
		keyError     = "error"
		keyComponent = "component"
		keyStack     = "stack"
		keyDuration  = "duration"
	)

	// Custom style for 'err' key
	styles.Keys[keyError] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorPink))
	styles.Values[keyError] = lipgloss.NewStyle().Bold(true)

	// Custom style for 'component' key
	styles.Keys[keyComponent] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorPink))

	// Custom style for 'stack' key
	styles.Keys[keyStack] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBlue))

	// Custom style for 'duration' key
	styles.Keys[keyDuration] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
}

// Error logs an error message with context.
func Error(message string, keyvals ...interface{}) {
	helperLogger.Error(message, keyvals...)
}

// Warn logs a warning message with context.
func Warn(message string, keyvals ...interface{}) {
	helperLogger.Warn(message, keyvals...)
}

// Info logs an informational message with context.
func Info(message string, keyvals ...interface{}) {
	helperLogger.Info(message, keyvals...)
}

// Debug logs a debug message with context.
func Debug(message string, keyvals ...interface{}) {
	helperLogger.Debug(message, keyvals...)
}

// Fatal logs an error message and exits with status code 1.
func Fatal(message string, keyvals ...interface{}) {
	helperLogger.Fatal(message, keyvals...)
}
