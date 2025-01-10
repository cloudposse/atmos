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

	switch LogLevel(logLevel) { // Convert logLevel to type LogLevel
	case LogLevelTrace:
		return LogLevelTrace, nil
	case LogLevelDebug:
		return LogLevelDebug, nil
	case LogLevelInfo:
		return LogLevelInfo, nil
	case LogLevelWarning:
		return LogLevelWarning, nil
	default:
		return LogLevelInfo, fmt.Errorf("invalid log level '%s'. Supported log levels are Trace, Debug, Info, Warning, Off", logLevel)
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
	if err != nil {
		_, err2 := theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}
	}
}

func (l *Logger) Trace(message string) {
	if l.LogLevel == LogLevelTrace {
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug {

		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Info(message string) {
	if l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug ||
		l.LogLevel == LogLevelInfo {

		l.log(theme.Colors.Default, message)
	}
}

func (l *Logger) Warning(message string) {
	if l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug ||
		l.LogLevel == LogLevelInfo ||
		l.LogLevel == LogLevelWarning {

		l.log(theme.Colors.Warning, message)
	}
}
