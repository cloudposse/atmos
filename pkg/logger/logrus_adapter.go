package logger

import (
	"io"
	"strings"

	"github.com/sirupsen/logrus"
)

// logrusAdapter adapts logrus output to Atmos's charmbracelet/log logger.
// This allows third-party libraries using logrus to have their logs formatted
// consistently with Atmos's logging style.
type logrusAdapter struct {
	io.Writer
}

// newLogrusAdapter creates a new logrus adapter that forwards to Atmos logger.
func newLogrusAdapter() *logrusAdapter {
	return &logrusAdapter{}
}

// Write implements io.Writer interface and forwards logrus output to Atmos logger.
// Logrus outputs formatted log lines like "level=info msg=\"message text\" key=value".
// This implementation preserves the original log level to ensure errors and warnings
// are not silently dropped when Atmos runs at Info level (the default).
func (a *logrusAdapter) Write(p []byte) (n int, err error) {
	// Convert bytes to string and trim trailing newline that logrus adds.
	message := strings.TrimSuffix(string(p), "\n")
	lower := strings.ToLower(message)

	// Parse and preserve the original log level from the formatted message.
	// This ensures critical diagnostics (errors, warnings) are visible at default log levels.
	switch {
	case strings.Contains(lower, "level=fatal"), strings.Contains(lower, "level=panic"), strings.Contains(lower, "level=error"):
		Error(message)
	case strings.Contains(lower, "level=warn"):
		Warn(message)
	case strings.Contains(lower, "level=debug"), strings.Contains(lower, "level=trace"):
		Debug(message)
	default:
		// Default to Info for unrecognized levels or info-level messages.
		Info(message)
	}

	return len(p), nil
}

// ConfigureLogrusForAtmos configures logrus to use Atmos logger instead of stdout.
func ConfigureLogrusForAtmos() {
	// Set logrus to output to our adapter.
	logrus.SetOutput(newLogrusAdapter())

	// Set logrus to use plain text formatter (not JSON) for better readability.
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true, // Atmos logger adds timestamps.
		DisableColors:    true, // Atmos logger handles colors.
		DisableQuote:     true, // Don't quote log messages.
	})

	// Set logrus level to match Atmos's current log level.
	logrus.SetLevel(atmosLevelToLogrus(GetLevel()))
}

// atmosLevelToLogrus converts Atmos log level to logrus log level.
func atmosLevelToLogrus(level Level) logrus.Level {
	switch level {
	case TraceLevel, DebugLevel:
		return logrus.DebugLevel
	case InfoLevel:
		return logrus.InfoLevel
	case WarnLevel:
		return logrus.WarnLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case FatalLevel:
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}
