package logger

import (
	"fmt"
	"os"

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

// logLevelOrder defines the order of log levels from most verbose to least verbose.
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
	charm    *log.Logger
}

func InitializeLogger(logLevel LogLevel, file string) (*Logger, error) {
	charm := GetCharmLogger()

	// Set log level
	charmLevel := log.InfoLevel
	switch logLevel {
	case LogLevelTrace:
		charmLevel = log.DebugLevel // Charmbracelet doesn't have Trace, use Debug.
	case LogLevelDebug:
		charmLevel = log.DebugLevel
	case LogLevelInfo:
		charmLevel = log.InfoLevel
	case LogLevelWarning:
		charmLevel = log.WarnLevel
	case LogLevelOff:
		charmLevel = log.FatalLevel + 1 // Set to a level higher than any defined level.
	}
	charm.SetLevel(charmLevel)

	if shouldUseCustomLogFile(file) {
		logFile, err := openLogFile(file)
		if err != nil {
			return nil, err
		}
		charm = GetCharmLoggerWithOutput(logFile)
		charm.SetLevel(charmLevel)
	}

	return &Logger{
		LogLevel: logLevel,
		File:     file,
		charm:    charm,
	}, nil
}

// InitializeLoggerFromCliConfig creates a logger based on Atmos CLI configuration.
func InitializeLoggerFromCliConfig(cfg *schema.AtmosConfiguration) (*Logger, error) {
	logLevel, err := ParseLogLevel(cfg.Logs.Level)
	if err != nil {
		return nil, err
	}
	return InitializeLogger(logLevel, cfg.Logs.File)
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
	return nil
}

func (l *Logger) Error(err error) {
	if err != nil && l.LogLevel != LogLevelOff {
		l.charm.Error("Error occurred", "error", err)

		_, err2 := theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}
	}
}

// shouldUseCustomLogFile returns true if a custom log file should be used instead of standard streams.
func shouldUseCustomLogFile(file string) bool {
	return file != "" && file != "/dev/stdout" && file != "/dev/stderr"
}

// FilePermDefault is the default permission for log files (0644 in octal) TODO: refactor this later
const FilePermDefault = 0o644

// openLogFile opens a log file for writing with appropriate flags.
func openLogFile(file string) (*os.File, error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, FilePermDefault)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	return f, nil
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
		// Charmbracelet doesn't have Trace level, use Debug with 'trace' context.
		l.charm.Debug(message, "level", "trace")
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.charm.Debug(message)
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.charm.Info(message)
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.charm.Warn(message)
		l.log(theme.Colors.Warning, message)
	}
}
