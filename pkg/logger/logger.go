package logger

import (
	"fmt"
	"os"
	"strings"

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

func NewLoggerFromCliConfig(config schema.AtmosConfiguration) (*Logger, error) {
	// Check for environment variable override
	if envLevel := os.Getenv("ATMOS_LOGS_LEVEL"); envLevel != "" {
		if _, err := ParseLogLevel(envLevel); err != nil {
			return nil, fmt.Errorf("Error: Invalid log level '%s'. Valid options are: [Trace Debug Info Warning Off]", envLevel)
		}
		config.Logs.Level = envLevel
	}

	// If no level is set in config or env, default to Info
	if config.Logs.Level == "" {
		config.Logs.Level = string(LogLevelInfo)
	} else {
		// Validate the config log level
		if _, err := ParseLogLevel(config.Logs.Level); err != nil {
			return nil, fmt.Errorf("Error: Invalid log level '%s'. Valid options are: [Trace Debug Info Warning Off]", config.Logs.Level)
		}
	}

	return NewLogger(LogLevel(config.Logs.Level), config.Logs.File)
}

func ParseLogLevel(level string) (LogLevel, error) {
	if level == "" {
		return "", fmt.Errorf("Error: Invalid log level ''. Valid options are: [Trace Debug Info Warning Off]")
	}

	// Convert to title case for consistent comparison
	normalizedLevel := strings.Title(strings.ToLower(level))

	switch normalizedLevel {
	case string(LogLevelTrace):
		return LogLevelTrace, nil
	case string(LogLevelDebug):
		return LogLevelDebug, nil
	case string(LogLevelInfo):
		return LogLevelInfo, nil
	case string(LogLevelWarning):
		return LogLevelWarning, nil
	case string(LogLevelOff):
		return LogLevelOff, nil
	default:
		return "", fmt.Errorf("Error: Invalid log level '%s'. Valid options are: [Trace Debug Info Warning Off]", level)
	}
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
			f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
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
	return nil
}

func (l *Logger) Error(err error) {
	if err != nil && l.LogLevel != LogLevelOff {
		_, err2 := theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
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
		l.log(theme.Colors.Info, fmt.Sprintf("[TRACE] %s", message))
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.log(theme.Colors.Info, fmt.Sprintf("[DEBUG] %s", message))
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.log(theme.Colors.Info, fmt.Sprintf("[INFO] %s", message))
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.log(theme.Colors.Warning, fmt.Sprintf("[WARNING] %s", message))
	}
}
