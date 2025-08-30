package logger

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
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

// logLevelOrder defines the order of log levels from most verbose to least verbose
var logLevelOrder = map[LogLevel]int{
	LogLevelTrace:   0,
	LogLevelDebug:   1,
	LogLevelInfo:    2,
	LogLevelWarning: 3,
	LogLevelOff:     4,
}

type Logger struct {
	LogLevel LogLevel
	File     string
}

func NewLogger(logLevel LogLevel, file string) (*Logger, error) {
	return &Logger{
		LogLevel: logLevel,
		File:     file,
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

	return "", fmt.Errorf("invalid log level `%s`. Valid options are: %v", logLevel, validLevels)
}

func (l *Logger) log(style *lipgloss.Style, message string) {
	// Apply style to the message
	styledMessage := style.Render(message)

	if l.File != "" {
		if l.File == "/dev/stdout" {
			fmt.Fprintln(os.Stdout, styledMessage)
		} else if l.File == "/dev/stderr" {
			fmt.Fprintln(os.Stderr, styledMessage)
		} else {
			// For regular files, write without styling
			f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				// Use theme error style for error messages
				errorStyle := theme.GetErrorStyle()
				fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Error opening log file: %s", err)))
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					errorStyle := theme.GetErrorStyle()
					fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Error closing log file: %s", err)))
				}
			}(f)

			// Write plain message to file (no styling)
			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				errorStyle := theme.GetErrorStyle()
				fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Error writing to log file: %s", err)))
			}
		}
	} else {
		fmt.Fprintln(os.Stdout, styledMessage)
	}
}

func (l *Logger) SetLogLevel(logLevel LogLevel) error {
	l.LogLevel = logLevel
	return nil
}

func (l *Logger) Error(err error) {
	if err != nil && l.LogLevel != LogLevelOff {
		errorStyle := theme.GetErrorStyle()
		_, err2 := fmt.Fprintln(color.Error, errorStyle.Render(err.Error()))
		if err2 != nil {
			// Fallback error handling
			fmt.Fprintln(os.Stderr, errorStyle.Render("Error logging the error:"))
			fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("%s", err2)))
			fmt.Fprintln(os.Stderr, errorStyle.Render("Original error:"))
			fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("%s", err)))
		}
	}
}

// isLevelEnabled checks if a given log level should be enabled based on the logger's current level
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	if l.LogLevel == LogLevelOff {
		return false
	}
	return logLevelOrder[level] >= logLevelOrder[l.LogLevel]
}

func (l *Logger) Trace(message string) {
	if l.isLevelEnabled(LogLevelTrace) {
		style := theme.GetTraceStyle()
		l.log(&style, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		style := theme.GetDebugStyle()
		l.log(&style, message)
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		style := theme.GetInfoStyle()
		l.log(&style, message)
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		style := theme.GetWarningStyle()
		l.log(&style, message)
	}
}
