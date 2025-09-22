package telemetry

import (
	"fmt"
	"io"

	log "github.com/charmbracelet/log"
)

// PosthogLogger is an adapter that implements the posthog.Logger interface
// using Atmos's charmbracelet/log. This ensures PostHog messages are properly
// integrated with Atmos logging and respect log levels. It also prevents
// PostHog errors from being printed directly to stdout/stderr.
type PosthogLogger struct{}

// NewPosthogLogger creates a new PosthogLogger instance.
func NewPosthogLogger() *PosthogLogger {
	return &PosthogLogger{}
}

// Debugf logs debug messages from PostHog using Atmos's logger.
func (p *PosthogLogger) Debugf(format string, args ...interface{}) {
	// Convert printf-style to structured logging
	msg := fmt.Sprintf(format, args...)
	log.Debug("PostHog debug message", "message", msg)
}

// Logf logs info messages from PostHog using Atmos's logger.
func (p *PosthogLogger) Logf(format string, args ...interface{}) {
	// Convert printf-style to structured logging at debug level
	msg := fmt.Sprintf(format, args...)
	log.Debug("PostHog info message", "message", msg)
}

// Warnf logs warning messages from PostHog using Atmos's logger.
func (p *PosthogLogger) Warnf(format string, args ...interface{}) {
	// Convert printf-style to structured logging
	msg := fmt.Sprintf(format, args...)
	log.Warn("PostHog warning", "message", msg)
}

// Errorf logs error messages from PostHog using Atmos's logger.
// This prevents PostHog errors from being printed directly to stderr.
func (p *PosthogLogger) Errorf(format string, args ...interface{}) {
	// Only log PostHog errors at debug level to avoid polluting user output.
	// Telemetry failures should not impact the user experience.
	msg := fmt.Sprintf(format, args...)
	log.Debug("PostHog telemetry error", "error", msg)
}

// SilentLogger is a no-op logger that discards all PostHog messages.
// This can be used when we want to completely suppress PostHog output.
type SilentLogger struct{}

// NewSilentLogger creates a new SilentLogger instance.
func NewSilentLogger() *SilentLogger {
	return &SilentLogger{}
}

// Debugf discards debug messages.
func (s *SilentLogger) Debugf(format string, args ...interface{}) {
}

// Logf discards info messages.
func (s *SilentLogger) Logf(format string, args ...interface{}) {
}

// Warnf discards warning messages.
func (s *SilentLogger) Warnf(format string, args ...interface{}) {
}

// Errorf discards error messages.
func (s *SilentLogger) Errorf(format string, args ...interface{}) {
}

// DiscardLogger is a logger that writes all output to io.Discard.
// This ensures PostHog doesn't write directly to stdout/stderr.
type DiscardLogger struct {
	writer io.Writer
}

// NewDiscardLogger creates a new DiscardLogger instance.
func NewDiscardLogger() *DiscardLogger {
	return &DiscardLogger{writer: io.Discard}
}

// Debugf discards debug messages.
func (d *DiscardLogger) Debugf(format string, args ...interface{}) {
}

// Logf discards info messages.
func (d *DiscardLogger) Logf(format string, args ...interface{}) {
}

// Warnf discards warning messages.
func (d *DiscardLogger) Warnf(format string, args ...interface{}) {
}

// Errorf discards error messages.
func (d *DiscardLogger) Errorf(format string, args ...interface{}) {
}
