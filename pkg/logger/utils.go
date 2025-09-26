package logger

import (
	"errors"
	"fmt"
	"math"
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
)

// ErrInvalidLogLevel is returned when an invalid log level is provided.
var ErrInvalidLogLevel = errors.New("invalid log level")

// ParseLogLevel parses a string log level and returns a LogLevel.
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
	case LogLevelOff:
		return Level(math.MaxInt32)
	default:
		return WarnLevel
	}
}
