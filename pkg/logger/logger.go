package logger

import (
	"fmt"
	"io"
	"os"

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
)

// AtmosTraceLevel is a custom log level one level lower than DebugLevel for more verbose logging.
const AtmosTraceLevel log.Level = log.DebugLevel - 1

// logLevelOrder defines the order of log levels from most verbose to least verbose.
var logLevelOrder = map[LogLevel]int{
	LogLevelTrace:   0,
	LogLevelDebug:   1,
	LogLevelInfo:    2,
	LogLevelWarning: 3,
	LogLevelOff:     4,
}

type Logger struct {
	LogLevel     LogLevel
	File         string
	StyledLogger *log.Logger
}

func NewLogger(logLevel LogLevel, file string) (*Logger, error) {
	// Determine the output writer based on file path.
	var writer io.Writer = os.Stdout
	if file == "/dev/stderr" {
		writer = os.Stderr
	} else if file != "" && file != "/dev/stdout" {
		// For actual files, we'll use a file writer.
		var err error
		writer, err = os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
	}

	// Create the styled logger.
	styledLogger := NewStyledLogger(writer)

	// Set the appropriate log level.
	switch logLevel {
	case LogLevelTrace:
		styledLogger.SetLevel(AtmosTraceLevel)
	case LogLevelDebug:
		styledLogger.SetLevel(log.DebugLevel)
	case LogLevelInfo:
		styledLogger.SetLevel(log.InfoLevel)
	case LogLevelWarning:
		styledLogger.SetLevel(log.WarnLevel)
	case LogLevelOff:
		styledLogger.SetLevel(log.ErrorLevel)
	}

	return &Logger{
		LogLevel:     logLevel,
		File:         file,
		StyledLogger: styledLogger,
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

	validLevels := []LogLevel{LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelOff}
	for _, level := range validLevels {
		if LogLevel(logLevel) == level {
			return level, nil
		}
	}

	return "", fmt.Errorf("Invalid log level `%s`. Valid options are: %v", logLevel, validLevels)
}

func (l *Logger) log(logColor *color.Color, message string) {
	if l.File != "" {
		if l.File == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else if l.File == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				color.Red("%s\n", err)
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					color.Red("%s\n", err)
				}
			}(f)

			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				color.Red("%s\n", err)
			}
		}
	} else {
		_, err := logColor.Fprintln(os.Stdout, message)
		if err != nil {
			color.Red("%s\n", err)
		}
	}
}

func (l *Logger) SetLogLevel(logLevel LogLevel) error {
	l.LogLevel = logLevel

	if l.StyledLogger != nil {
		switch logLevel {
		case LogLevelTrace:
			l.StyledLogger.SetLevel(AtmosTraceLevel)
		case LogLevelDebug:
			l.StyledLogger.SetLevel(log.DebugLevel)
		case LogLevelInfo:
			l.StyledLogger.SetLevel(log.InfoLevel)
		case LogLevelWarning:
			l.StyledLogger.SetLevel(log.WarnLevel)
		case LogLevelOff:
			l.StyledLogger.SetLevel(log.ErrorLevel)
		}
	}

	return nil
}

func (l *Logger) Error(err error) {
	if err != nil && l.LogLevel != LogLevelOff {
		l.StyledLogger.Error("Error occurred", "error", err)
	}
}

// isLevelEnabled checks if a given log level should be enabled based on the logger's current level.
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	if l.LogLevel == LogLevelOff {
		return false
	}
	return logLevelOrder[level] >= logLevelOrder[l.LogLevel]
}

func (l *Logger) Trace(message string) {
	if l.isLevelEnabled(LogLevelTrace) {
		l.StyledLogger.Log(AtmosTraceLevel, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.StyledLogger.Debug(message)
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.StyledLogger.Info(message)
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.StyledLogger.Warn(message)
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

// NewStyledLogger creates a new styled Charmbracelet logger using Atmos theme colors.
func NewAtmosLogger(writer io.Writer) *log.Logger {
	// Create styles based on Atmos theme.
	styles := log.DefaultStyles()

	// Set level styles.
	styles.Levels[log.ErrorLevel] = createLevelStyle(errorLevelLabel, theme.ColorRed)
	styles.Levels[log.WarnLevel] = createLevelStyle(warnLevelLabel, theme.ColorRed)
	styles.Levels[log.InfoLevel] = createLevelStyle(infoLevelLabel, theme.ColorCyan)
	styles.Levels[log.DebugLevel] = createLevelStyle(debugLevelLabel, theme.ColorBlue)
	styles.Levels[AtmosTraceLevel] = createLevelStyle(traceLevelLabel, theme.ColorPink)

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
