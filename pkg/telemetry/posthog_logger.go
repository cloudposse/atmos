package telemetry

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// PosthogLogger is an adapter that implements the posthog.Logger interface
// using Atmos's charmbracelet/log. This ensures PostHog messages are properly
// integrated with Atmos logging and respect log levels. It also prevents
// PostHog errors from being printed directly to stdout/stderr.
type PosthogLogger struct {
	logger *log.AtmosLogger
}

// NewPosthogLogger creates a new PosthogLogger instance.
func NewPosthogLogger() *PosthogLogger {
	// Create a logger with PostHog prefix
	// Use the default logger instance which respects global log level
	return &PosthogLogger{logger: log.Default().WithPrefix("posthog")}
}

// Debugf logs debug messages from PostHog using Atmos's logger.
func (p *PosthogLogger) Debugf(format string, args ...interface{}) {
	// Convert printf-style to structured logging
	msg := fmt.Sprintf(format, args...)
	p.logger.Debug(msg)
}

// Logf logs info messages from PostHog using Atmos's logger.
// PostHog uses Logf for INFO level messages, but we log them at DEBUG
// to reduce noise in production.
func (p *PosthogLogger) Logf(format string, args ...interface{}) {
	// Convert printf-style to structured logging at debug level
	// to avoid cluttering user output with telemetry info
	msg := fmt.Sprintf(format, args...)
	p.logger.Debug(msg)
}

// Warnf logs warning messages from PostHog using Atmos's logger.
func (p *PosthogLogger) Warnf(format string, args ...interface{}) {
	// Convert printf-style to structured logging
	msg := fmt.Sprintf(format, args...)
	p.logger.Warn(msg)
}

// Errorf logs error messages from PostHog using Atmos's logger.
// This prevents PostHog errors from being printed directly to stderr.
func (p *PosthogLogger) Errorf(format string, args ...interface{}) {
	// Only log PostHog errors at debug level to avoid polluting user output.
	// Telemetry failures should not impact the user experience.
	msg := fmt.Sprintf(format, args...)
	p.logger.Debug(msg, "posthog_level", "error")
}

// Infof logs info messages from PostHog using Atmos's logger.
// PostHog uses Infof for INFO level messages, but we log them at DEBUG
// to reduce noise in production.
func (p *PosthogLogger) Infof(format string, args ...interface{}) {
	// Convert printf-style to structured logging at debug level
	// to avoid cluttering user output with telemetry info
	msg := fmt.Sprintf(format, args...)
	p.logger.Debug(msg)
}

// Printf logs messages from PostHog using Atmos's logger.
// This is a generic print method that PostHog may call.
// We log at DEBUG level to avoid cluttering user output.
func (p *PosthogLogger) Printf(format string, args ...interface{}) {
	// Convert printf-style to structured logging at debug level
	msg := fmt.Sprintf(format, args...)
	p.logger.Debug(msg)
}

// SilentLogger is a no-op logger that discards all PostHog messages.
// It satisfies the posthog.Logger interface but doesn't output anything.
// This is used when telemetry.posthog_logging is disabled to completely
// suppress PostHog internal messages.
type SilentLogger struct{}

// NewSilentLogger creates a new SilentLogger instance.
func NewSilentLogger() *SilentLogger {
	return &SilentLogger{}
}

// Debugf implements posthog.Logger but does nothing.
func (s *SilentLogger) Debugf(format string, args ...interface{}) {
}

// Logf implements posthog.Logger but does nothing.
func (s *SilentLogger) Logf(format string, args ...interface{}) {
}

// Warnf implements posthog.Logger but does nothing.
func (s *SilentLogger) Warnf(format string, args ...interface{}) {
}

// Errorf implements posthog.Logger but does nothing.
func (s *SilentLogger) Errorf(format string, args ...interface{}) {
}

// Infof implements posthog.Logger but does nothing.
func (s *SilentLogger) Infof(format string, args ...interface{}) {
}

// Printf implements posthog.Logger but does nothing.
func (s *SilentLogger) Printf(format string, args ...interface{}) {
}
