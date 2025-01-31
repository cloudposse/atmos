package logger

import (
	"fmt"
	"os"

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

	return "", fmt.Errorf("Error: Invalid log level `%s`. Valid options are: %v", logLevel, validLevels)
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
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.log(theme.Colors.Warning, message)
	}
}
