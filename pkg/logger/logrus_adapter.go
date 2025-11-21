package logger

import (
	"encoding/json"
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
// Logrus outputs JSON formatted log lines when configured with JSONFormatter.
// This implementation parses the JSON to preserve the original log level and structured fields,
// ensuring errors and warnings are not silently dropped when Atmos runs at Info level (the default).
func (a *logrusAdapter) Write(p []byte) (n int, err error) {
	// Convert bytes to string and trim trailing newline that logrus adds.
	message := strings.TrimSuffix(string(p), "\n")

	// Try to parse as JSON for structured logging.
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(message), &entry); err == nil {
		// Successfully parsed JSON - extract level and fields.
		level, _ := entry["level"].(string)
		msg, _ := entry["msg"].(string)

		// Build structured fields for Atmos logger.
		// Exclude standard fields (level, msg, time) and pass the rest as key-value pairs.
		var fields []interface{}
		for k, v := range entry {
			if k != "level" && k != "msg" && k != "time" {
				fields = append(fields, k, v)
			}
		}

		// Route to appropriate log level based on parsed level field.
		switch strings.ToLower(level) {
		case "fatal", "panic", "error":
			Error(msg, fields...)
		case "warning", "warn":
			Warn(msg, fields...)
		case "debug", "trace":
			Debug(msg, fields...)
		default:
			Info(msg, fields...)
		}
	} else {
		// Fallback: Not JSON (shouldn't happen with JSONFormatter, but handle gracefully).
		// Log the raw message at Info level.
		Info(message)
	}

	return len(p), nil
}

// ConfigureLogrusForAtmos configures logrus to use Atmos logger instead of stdout.
// Uses JSONFormatter for structured logging, allowing us to parse and preserve
// log levels and structured fields from logrus entries.
func ConfigureLogrusForAtmos() {
	// Set logrus to output to our adapter.
	logrus.SetOutput(newLogrusAdapter())

	// Set logrus to use JSON formatter for structured logging.
	// This allows us to parse the output and preserve log levels and structured fields.
	logrus.SetFormatter(&logrus.JSONFormatter{
		DisableTimestamp: true, // Atmos logger adds timestamps.
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
