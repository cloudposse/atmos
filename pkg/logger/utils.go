package logger

import (
	"errors"
	"fmt"
	"math"
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

	return "", fmt.Errorf("%w: `%s`. Valid options are: %v", ErrInvalidLogLevel, logLevel, validLevels)
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
		return Level(math.MaxInt32)
	default:
		return InfoLevel
	}
}
