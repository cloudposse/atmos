package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const FilePermission = 0o644

// AtmosTraceLevel is a custom log level one level lower than DebugLevel for more verbose logging.
const AtmosTraceLevel log.Level = log.DebugLevel - 1

// LogLevelStrings maps string level names to corresponding charmbracelet log levels for compatibility.
var LogLevelStrings = map[string]log.Level{
	"Trace":   AtmosTraceLevel,
	"Debug":   log.DebugLevel,
	"Info":    log.InfoLevel,
	"Warning": log.WarnLevel,
	"Error":   log.ErrorLevel,
	"Off":     log.FatalLevel + 1, // Higher than Fatal to effectively disable logging
}

// ErrUnsupportedDeviceFile represents an error when an unsupported device file is specified.
type ErrUnsupportedDeviceFile struct {
	file string
}

// ErrInvalidLogLevel is returned when an invalid log level is provided.
var ErrInvalidLogLevel = errors.New("Invalid log level")

// Error returns the error message for an unsupported device file.
func (e ErrUnsupportedDeviceFile) Error() string {
	return fmt.Sprintf("unsupported device file: %s", e.file)
}

type AtmosLogger struct {
	*log.Logger
}

// validateLogDestination validates the log destination path.
// Returns an error if the path is a device file not in the allowed list.
func validateLogDestination(file string) error {
	// Skip validation for empty or standard allowed device files
	if file == "" || file == "/dev/stderr" || file == "/dev/null" {
		return nil
	}

	// Special warning for stdout but still allow it
	if file == "/dev/stdout" {
		return nil
	}

	// Reject other device files
	if strings.HasPrefix(file, "/dev/") {
		return ErrUnsupportedDeviceFile{file: file}
	}

	return nil
}

// getLogWriter returns an appropriate writer based on the file path.
// It handles special paths like /dev/stdout, /dev/stderr, and /dev/null.
func getLogWriter(file string) (io.Writer, error) {
	// Default to stderr
	var writer io.Writer = os.Stderr

	switch file {
	case "":
		// Keep the default (stderr)
		return writer, nil
	case "/dev/stdout":
		log.Warn("Sending logs to stdout will break commands that rely on Atmos output")
		return os.Stdout, nil
	case "/dev/stderr":
		return os.Stderr, nil
	case "/dev/null":
		return io.Discard, nil
	default:
		writer, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, FilePermission)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		return writer, nil
	}
}

func NewLogger(logLevel log.Level, file string) (*AtmosLogger, error) {
	// Validate the log destination
	if err := validateLogDestination(file); err != nil {
		return nil, err
	}

	// Get the appropriate writer based on file path
	writer, err := getLogWriter(file)
	if err != nil {
		return nil, err
	}

	// Create the Atmos logger with styled output
	logger := NewAtmosLogger(writer)

	// Set the log level directly
	logger.SetLevel(logLevel)

	return &AtmosLogger{
		Logger: logger,
	}, nil
}

func NewLoggerFromCliConfig(cfg *schema.AtmosConfiguration) (*AtmosLogger, error) {
	logLevel, err := ParseLogLevel(cfg.Logs.Level)
	if err != nil {
		return nil, err
	}
	return NewLogger(logLevel, cfg.Logs.File)
}

// ParseLogLevel parses a string log level and returns the corresponding log.Level.
func ParseLogLevel(logLevel string) (log.Level, error) {
	if logLevel == "" {
		return log.InfoLevel, nil
	}

	if level, ok := LogLevelStrings[logLevel]; ok {
		return level, nil
	}

	validLevels := []string{"Trace", "Debug", "Info", "Warning", "Off"}

	return 0, fmt.Errorf("%w `%s`. Valid options are: %v", ErrInvalidLogLevel, logLevel, validLevels)
}

func (l *AtmosLogger) SetLogLevel(logLevel log.Level) error {
	l.SetLevel(logLevel)
	return nil
}

// GetLevel returns the current log level of the logger.
func (l *AtmosLogger) GetLevel() log.Level {
	return l.Logger.GetLevel()
}

func (l *AtmosLogger) Error(err error) {
	if l.GetLevel() <= log.ErrorLevel {
		l.Logger.Error("Error occurred", "error", err)
	}
}

func (l *AtmosLogger) Trace(message string) {
	if l.GetLevel() <= AtmosTraceLevel {
		l.Logger.Log(AtmosTraceLevel, message)
	}
}

func (l *AtmosLogger) Debug(message string) {
	if l.GetLevel() <= log.DebugLevel {
		l.Logger.Debug(message)
	}
}

func (l *AtmosLogger) Info(message string) {
	if l.GetLevel() <= log.InfoLevel {
		l.Logger.Info(message)
	}
}

// Warn logs a warning message using the standard Charmbracelet logger naming convention.
func (l *AtmosLogger) Warn(message string) {
	if l.GetLevel() <= log.WarnLevel {
		l.Logger.Warn(message)
	}
}

// Constants for logger configuration.
const (
	// Log level labels.
	errorLevelLabel = "ERRO"
	warnLevelLabel  = "WARN"
	infoLevelLabel  = "INFO"
	debugLevelLabel = "DEBU"
	traceLevelLabel = "TRCE"
)

// createLevelStyle creates a formatted styled log level with maximum visibility.
func createLevelStyle(label string, bgColor string) lipgloss.Style {
	return lipgloss.NewStyle().
		SetString(label).
		Padding(0, 0, 0, 0).
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Bold(true).
		Border(lipgloss.NormalBorder(), false, false, false, false)
}

// NewAtmosLogger creates a new Charmbracelet logger styled with Atmos theme colors.
func NewAtmosLogger(writer io.Writer) *log.Logger {
	// Create styles based on Atmos theme.
	styles := log.DefaultStyles()

	// Set level styles.
	styles.Levels[log.ErrorLevel] = createLevelStyle(errorLevelLabel, theme.ColorRed)
	styles.Levels[log.WarnLevel] = createLevelStyle(warnLevelLabel, theme.ColorRed)
	styles.Levels[log.InfoLevel] = createLevelStyle(infoLevelLabel, theme.ColorCyan)
	styles.Levels[log.DebugLevel] = createLevelStyle(debugLevelLabel, theme.ColorBlue)
	styles.Levels[AtmosTraceLevel] = createLevelStyle(traceLevelLabel, theme.ColorPink)

	// Default style for all keys.
	styles.Keys[""] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))

	// Style for error values.
	styles.Keys["error"] = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	styles.Values["error"] = lipgloss.NewStyle().Bold(true)

	// Common key styles using a map for concise declaration.
	keyColors := map[string]string{
		"component": theme.ColorPink,
		"stack":     theme.ColorGreen,
		"duration":  theme.ColorGreen,
	}

	for key, color := range keyColors {
		styles.Keys[key] = lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	}

	// Create the logger with our styles.
	logger := log.New(writer)
	logger.SetStyles(styles)

	return logger
}
