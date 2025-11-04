package logger

import (
	"io"

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
func (a *logrusAdapter) Write(p []byte) (n int, err error) {
	// Convert bytes to string and trim trailing newline that logrus adds.
	message := string(p)
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}

	// Log at debug level since these are internal library messages.
	// We don't want to parse logrus's structured format - just pass through the message.
	Debug(message)

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

	// Set logrus level to Info to avoid excessive debug logs from saml2aws.
	// Users can see detailed SAML flow via Atmos's own debug logs.
	logrus.SetLevel(logrus.InfoLevel)
}
