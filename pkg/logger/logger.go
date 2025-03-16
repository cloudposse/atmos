package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

type LogLevel string

const (
	LogLevelOff     LogLevel = "Off"
	LogLevelTrace   LogLevel = "Trace"
	LogLevelDebug   LogLevel = "Debug"
	LogLevelInfo    LogLevel = "Info"
	LogLevelWarning LogLevel = "Warning"
	LogLevelError   LogLevel = "Error"
)

const FilePermission = 0o644

// AtmosTraceLevel is a custom log level one level lower than DebugLevel for more verbose logging.
const AtmosTraceLevel log.Level = log.DebugLevel - 1

// logLevelOrder defines the order of log levels from most verbose to least verbose.
var logLevelOrder = map[LogLevel]int{
	LogLevelTrace:   0,
	LogLevelDebug:   1,
	LogLevelInfo:    2,
	LogLevelWarning: 3,
	LogLevelError:   4,
	LogLevelOff:     5,
}

// ErrUnsupportedDeviceFile represents an error when an unsupported device file is specified.
type ErrUnsupportedDeviceFile struct {
	file string
}

// Error returns the error message for an unsupported device file.
func (e ErrUnsupportedDeviceFile) Error() string {
	return fmt.Sprintf("unsupported device file: %s", e.file)
}

type Logger struct {
	LogLevel    LogLevel
	File        string
	AtmosLogger *log.Logger
}

// validateLogDestination validates the log destination path.
// Returns an error if the path is a device file not in the allowed list.
func validateLogDestination(file string) error {
	// Skip validation for empty or standard allowed device files
	if file == "" || file == "/dev/stdout" || file == "/dev/stderr" || file == "/dev/null" {
		return nil
	}

	// Reject other device files
	if strings.HasPrefix(file, "/dev/") {
		return ErrUnsupportedDeviceFile{file: file}
	}

	return nil
}

// For testing purposes
var logWarningFunc = func(message string) {
	log.Warn(message)
}

// logWarning is a simple logger for internal warnings.
func logWarning(message string) {
	logWarningFunc(message)
}

// getLogWriter returns an appropriate writer based on the file path.
// It handles special paths like /dev/stdout, /dev/stderr, and /dev/null.
func getLogWriter(file string) (io.Writer, error) {
	switch file {
	case "":
		return os.Stdout, nil
	case "/dev/stdout":
		logWarning("WARNING: Using stdout for logs may interfere with JSON output parsing")
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

func NewLogger(logLevel LogLevel, file string) (*Logger, error) {
	// Validate the log destination
	if err := validateLogDestination(file); err != nil {
		return nil, err
	}

	// Get the appropriate writer based on file path
	writer, err := getLogWriter(file)
	if err != nil {
		return nil, err
	}

	// Create the Atmos logger
	atmosLogger := NewAtmosLogger(writer)

	// Set the appropriate log level
	switch logLevel {
	case LogLevelTrace:
		atmosLogger.SetLevel(AtmosTraceLevel)
	case LogLevelDebug:
		atmosLogger.SetLevel(log.DebugLevel)
	case LogLevelInfo:
		atmosLogger.SetLevel(log.InfoLevel)
	case LogLevelWarning:
		atmosLogger.SetLevel(log.WarnLevel)
	case LogLevelError:
		atmosLogger.SetLevel(log.ErrorLevel)
	case LogLevelOff:
		atmosLogger.SetLevel(log.FatalLevel)
	}

	return &Logger{
		LogLevel:    logLevel,
		File:        file,
		AtmosLogger: atmosLogger,
	}, nil
}

func NewLoggerFromCliConfig(cfg schema.AtmosConfiguration) (*Logger, error) {
	logLevel, err := ParseLogLevel(cfg.Logs.Level)
	if err != nil {
		return nil, err
	}
	return NewLogger(logLevel, cfg.Logs.File)
}

func ParseLogLevel(logLevel string) (LogLevel, error) {
	if logLevel == "" {
		return LogLevelInfo, nil
	}

	validLevels := []LogLevel{LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelError, LogLevelOff}
	for _, level := range validLevels {
		if LogLevel(logLevel) == level {
			return level, nil
		}
	}

	return "", fmt.Errorf("Invalid log level `%s`. Valid options are: %v", logLevel, validLevels)
}

// getLogOutputWriter returns the appropriate writer for logging based on the log file path.
// This is a helper for the log method to handle different output destinations.
func (l *Logger) getLogOutputWriter() (io.Writer, func(), error) {
	// Default to stdout
	if l.File == "" {
		return os.Stdout, func() {}, nil
	}

	switch l.File {
	case "/dev/stdout":
		return os.Stdout, func() {}, nil
	case "/dev/stderr":
		return os.Stderr, func() {}, nil
	case "/dev/null":
		return io.Discard, func() {}, nil
	default:
		// For actual files, open the file
		f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return nil, nil, err
		}

		// Return a cleanup function to close the file
		cleanup := func() {
			err := f.Close()
			if err != nil {
				color.Red("%s\n", err)
			}
		}

		return f, cleanup, nil
	}
}

func (l *Logger) log(logColor *color.Color, message string) {
	// Get the appropriate writer
	writer, cleanup, err := l.getLogOutputWriter()
	if err != nil {
		color.Red("%s\n", err)
		return
	}

	defer cleanup()

	if writer == io.Discard {
		return
	} else if stdWriter, ok := writer.(*os.File); ok && (stdWriter == os.Stdout || stdWriter == os.Stderr) {
		_, err = logColor.Fprintln(writer, message)
	} else {
		_, err = fmt.Fprintln(writer, message)
	}

	if err != nil {
		color.Red("%s\n", err)
	}
}

func (l *Logger) SetLogLevel(logLevel LogLevel) error {
	l.LogLevel = logLevel

	if l.AtmosLogger != nil {
		switch logLevel {
		case LogLevelTrace:
			l.AtmosLogger.SetLevel(AtmosTraceLevel)
		case LogLevelDebug:
			l.AtmosLogger.SetLevel(log.DebugLevel)
		case LogLevelInfo:
			l.AtmosLogger.SetLevel(log.InfoLevel)
		case LogLevelWarning:
			l.AtmosLogger.SetLevel(log.WarnLevel)
		case LogLevelError:
			l.AtmosLogger.SetLevel(log.ErrorLevel)
		case LogLevelOff:
			l.AtmosLogger.SetLevel(log.FatalLevel)
		}
	}

	return nil
}

// isLevelEnabled checks if a given log level should be enabled based on the logger's current level.
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	return logLevelOrder[level] >= logLevelOrder[l.LogLevel]
}

func (l *Logger) Error(err error) {
	if l.isLevelEnabled(LogLevelError) {
		l.AtmosLogger.Error("Error occurred", "error", err)
	}
}

func (l *Logger) Trace(message string) {
	if l.isLevelEnabled(LogLevelTrace) {
		l.AtmosLogger.Log(AtmosTraceLevel, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.AtmosLogger.Debug(message)
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.AtmosLogger.Info(message)
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.AtmosLogger.Warn(message)
	}
}

// Log level labels.
const (
	errorLevelLabel = "ERROR"
	warnLevelLabel  = "WARN"
	infoLevelLabel  = "INFO"
	debugLevelLabel = "DEBU"
	traceLevelLabel = "TRCE"
)

// createLevelStyle creates a formatted styled log level with maximum visibility.
func createLevelStyle(label string, bgColor string) lipgloss.Style {
	return lipgloss.NewStyle().
		SetString(label).
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Bold(true).                                                 // Make text bold for better visibility
		Border(lipgloss.NormalBorder(), false, false, false, false) // Add borders for better visibility
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
