package logger

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

// sensitiveLogValuePatterns matches common secret key=value or key:value pairs
// that third-party libraries (e.g. saml2aws) may include in log messages.
// Matched values are redacted before forwarding to the Atmos logger.
var sensitiveLogValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|session[_-]?id|credential)\s*[:=]\s*([^\s,;}"]+)`),
}

// sanitizeLogMessage redacts common sensitive key=value patterns before logging.
func sanitizeLogMessage(msg string) string {
	for _, pattern := range sensitiveLogValuePatterns {
		msg = pattern.ReplaceAllString(msg, `$1=[REDACTED]`)
	}
	return msg
}

// sanitizeFieldValue redacts the value if the field key looks like a secret.
// For non-sensitive keys with string values, it also applies sanitizeLogMessage
// to catch embedded sensitive patterns (e.g. a "details" field containing
// "password=s3cret").
func sanitizeFieldValue(key string, value interface{}) interface{} {
	if isSensitiveLogKey(key) {
		return "[REDACTED]"
	}
	// Also sanitize string values that may contain embedded sensitive data.
	if strVal, ok := value.(string); ok {
		return sanitizeLogMessage(strVal)
	}
	return value
}

// sensitiveKeySubstrings lists substrings that identify sensitive log field keys.
// Any key containing one of these (case-insensitive) has its value redacted.
var sensitiveKeySubstrings = []string{
	"password", "passwd", "pwd", "secret", "token",
	"apikey", "api_key", "authorization", "cookie",
	"session", "private_key", "client_secret", "credential",
}

// isSensitiveLogKey identifies keys that commonly hold secrets.
func isSensitiveLogKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, substr := range sensitiveKeySubstrings {
		if strings.Contains(k, substr) {
			return true
		}
	}
	return false
}

// levelLogger abstracts the level-dispatched logging calls that Write() uses.
// Production code uses the global Atmos logger via defaultLevelLogger; tests
// inject a recording implementation to assert level routing.
type levelLogger interface {
	Error(msg string, keyvals ...interface{})
	Warn(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Debug(msg string, keyvals ...interface{})
}

// atmosLevelLogger delegates to the package-level Atmos log functions.
type atmosLevelLogger struct{}

func (atmosLevelLogger) Error(msg string, kv ...interface{}) { Error(msg, kv...) }
func (atmosLevelLogger) Warn(msg string, kv ...interface{})  { Warn(msg, kv...) }
func (atmosLevelLogger) Info(msg string, kv ...interface{})  { Info(msg, kv...) }
func (atmosLevelLogger) Debug(msg string, kv ...interface{}) { Debug(msg, kv...) }

// logrusAdapter adapts logrus output to Atmos's charmbracelet/log logger.
// This allows third-party libraries using logrus to have their logs formatted
// consistently with Atmos's logging style.
type logrusAdapter struct {
	io.Writer
	logger levelLogger
}

// newLogrusAdapter creates a new logrus adapter that forwards to Atmos logger.
func newLogrusAdapter() *logrusAdapter {
	return &logrusAdapter{logger: atmosLevelLogger{}}
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
		msg = sanitizeLogMessage(msg)

		// Build structured fields for Atmos logger.
		// Exclude standard fields (level, msg, time) and pass the rest as key-value pairs.
		// Redact values for keys that look like secrets.
		var fields []interface{}
		for k, v := range entry {
			if k != "level" && k != "msg" && k != "time" {
				fields = append(fields, k, sanitizeFieldValue(k, v))
			}
		}

		// Route to appropriate log level based on parsed level field.
		switch strings.ToLower(level) {
		case "fatal", "panic", "error":
			a.logger.Error(msg, fields...)
		case "warning", "warn":
			a.logger.Warn(msg, fields...)
		case "debug", "trace":
			a.logger.Debug(msg, fields...)
		default:
			a.logger.Info(msg, fields...)
		}
	} else {
		// Fallback: Not JSON (shouldn't happen with JSONFormatter, but handle gracefully).
		// Do not log raw content to avoid leaking sensitive data from unstructured
		// messages — regex-based sanitization cannot catch every format.
		a.logger.Info("Received non-JSON logrus message (content omitted)", "length", len(message))
	}

	return len(p), nil
}

// ConfigureLogrusForAtmos configures logrus to use Atmos logger instead of stdout.
// Uses JSONFormatter for structured logging, allowing us to parse and preserve
// log levels and structured fields from logrus entries.
// Note: perf.Track is not used here because pkg/perf imports pkg/logger,
// which would create an import cycle. This is a one-time configuration
// call during auth setup, not a per-request hot path.
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
