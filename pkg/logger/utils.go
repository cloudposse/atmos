package logger

import (
	"errors"
	"fmt"
	"strings"
)

// LogLevel represents log level as a string.
type LogLevel string

const (
	// LogLevelOff turns off logging.
	LogLevelOff LogLevel = "Off"
	// LogLevelTrace is the trace log level.
	LogLevelTrace LogLevel = "Trace"
	// LogLevelDebug is the debug log level.
	LogLevelDebug LogLevel = "Debug"
	// LogLevelInfo is the info log level.
	LogLevelInfo LogLevel = "Info"
	// LogLevelWarning is the warning log level.
	LogLevelWarning LogLevel = "Warning"
	// LogLevelError is the error log level.
	LogLevelError LogLevel = "Error"
)

// ErrInvalidLogLevel is returned when an invalid log level is provided.
var ErrInvalidLogLevel = errors.New("invalid log level")

// ParseLogLevel parses a string log level and returns a LogLevel.
func ParseLogLevel(logLevel string) (LogLevel, error) {
	logLevel = strings.TrimSpace(logLevel)
	if logLevel == "" {
		return LogLevelInfo, nil
	}

	// Make case-insensitive comparison
	logLevelLower := strings.ToLower(logLevel)

	// Handle warn as alias for warning
	if logLevelLower == "warn" {
		return LogLevelWarning, nil
	}

	validLevels := []LogLevel{LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelError, LogLevelOff}
	for _, level := range validLevels {
		if strings.ToLower(string(level)) == logLevelLower {
			return level, nil
		}
	}

	// Return just the sentinel with details that can be parsed by the error builder.
	return "", fmt.Errorf("%w\nthe log level '%s' is not recognized. Valid options are: %v", ErrInvalidLogLevel, logLevel, validLevels)
}

// ConvertLogLevel converts a string LogLevel to a charm Level.
func ConvertLogLevel(level LogLevel) Level {
	switch level {
	case LogLevelTrace:
		return TraceLevel
	case LogLevelDebug:
		return DebugLevel
	case LogLevelInfo:
		return InfoLevel
	case LogLevelWarning:
		return WarnLevel
	case LogLevelError:
		return ErrorLevel
	case LogLevelOff:
		// Disable logging by setting level above FatalLevel.
		// charmbracelet/log only recognizes defined Level constants, so we use FatalLevel + 1
		// instead of math.MaxInt32 which would not work correctly.
		return FatalLevel + 1
	default:
		return InfoLevel
	}
}
