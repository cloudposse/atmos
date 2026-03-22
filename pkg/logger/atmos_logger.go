package logger

import (
	"io"
	"math"
	"os"
	"sync"

	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// AtmosLogger wraps the Charm Bracelet logger to provide a consistent interface for Atmos while maintaining full compatibility.
type AtmosLogger struct {
	charm  *charm.Logger
	mu     sync.RWMutex
	writer io.Writer // tracks the current output writer for GetOutput
}

// NewAtmosLogger creates a new AtmosLogger instance wrapping the given charm logger.
// An optional io.Writer may be provided as the second argument to initialize the
// tracked output writer. When omitted, os.Stderr is used as the default, which
// matches the default writer of both charm.Default() and charm.New(os.Stderr).
//
// If charmLogger was initialized with a non-stderr writer (e.g., charm.New(&buf)),
// pass that same writer as the second argument so that GetOutput() returns the
// correct value immediately without requiring a follow-up SetOutput call.
func NewAtmosLogger(charmLogger *charm.Logger, w ...io.Writer) *AtmosLogger {
	if charmLogger == nil {
		charmLogger = charm.Default()
	}
	writer := io.Writer(os.Stderr)
	if len(w) > 0 && w[0] != nil {
		writer = w[0]
	}
	return &AtmosLogger{charm: charmLogger, writer: writer}
}

// Trace logs a trace message.
func (l *AtmosLogger) Trace(msg interface{}, keyvals ...interface{}) {
	l.charm.Log(TraceLevel, msg, keyvals...)
}

// Tracef logs a formatted trace message.
func (l *AtmosLogger) Tracef(format string, args ...interface{}) {
	l.charm.Logf(TraceLevel, format, args...)
}

// Debug logs a debug message.
func (l *AtmosLogger) Debug(msg interface{}, keyvals ...interface{}) {
	l.charm.Debug(msg, keyvals...)
}

// Debugf logs a formatted debug message.
func (l *AtmosLogger) Debugf(format string, args ...interface{}) {
	l.charm.Debugf(format, args...)
}

// Info logs an info message.
func (l *AtmosLogger) Info(msg interface{}, keyvals ...interface{}) {
	l.charm.Info(msg, keyvals...)
}

// Infof logs a formatted info message.
func (l *AtmosLogger) Infof(format string, args ...interface{}) {
	l.charm.Infof(format, args...)
}

// Warn logs a warning message.
func (l *AtmosLogger) Warn(msg interface{}, keyvals ...interface{}) {
	l.charm.Warn(msg, keyvals...)
}

// Warnf logs a formatted warning message.
func (l *AtmosLogger) Warnf(format string, args ...interface{}) {
	l.charm.Warnf(format, args...)
}

// Error logs an error message.
func (l *AtmosLogger) Error(msg interface{}, keyvals ...interface{}) {
	l.charm.Error(msg, keyvals...)
}

// Errorf logs a formatted error message.
func (l *AtmosLogger) Errorf(format string, args ...interface{}) {
	l.charm.Errorf(format, args...)
}

// Fatal logs a fatal message and exits the program.
func (l *AtmosLogger) Fatal(msg interface{}, keyvals ...interface{}) {
	l.charm.Fatal(msg, keyvals...)
}

// Fatalf logs a formatted fatal message and exits the program.
func (l *AtmosLogger) Fatalf(format string, args ...interface{}) {
	l.charm.Fatalf(format, args...)
}

// Log logs a message at the given level.
func (l *AtmosLogger) Log(level Level, msg interface{}, keyvals ...interface{}) {
	l.charm.Log(level, msg, keyvals...)
}

// Logf logs a formatted message at the given level.
func (l *AtmosLogger) Logf(level Level, format string, args ...interface{}) {
	l.charm.Logf(level, format, args...)
}

// SetLevel sets the log level.
func (l *AtmosLogger) SetLevel(level Level) {
	l.charm.SetLevel(level)
}

// GetLevel returns the current log level.
func (l *AtmosLogger) GetLevel() Level {
	return l.charm.GetLevel()
}

// SetOutput sets the output writer.
// The write lock is held for both the tracked writer update and the underlying
// charm logger update so that any concurrent GetOutput call always observes a
// fully consistent pair: l.writer and charm's internal writer are always equal.
func (l *AtmosLogger) SetOutput(w io.Writer) {
	l.mu.Lock()
	l.writer = w
	l.charm.SetOutput(w)
	l.mu.Unlock()
}

// GetOutput returns the current output writer.
func (l *AtmosLogger) GetOutput() io.Writer {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.writer
}

// SetStyles sets the log styles.
func (l *AtmosLogger) SetStyles(styles *charm.Styles) {
	l.charm.SetStyles(styles)
}

// SetColorProfile sets the color profile.
func (l *AtmosLogger) SetColorProfile(profile termenv.Profile) {
	l.charm.SetColorProfile(profile)
}

// WithPrefix returns a new logger with the given prefix.
// The output writer is propagated to the child logger so that GetOutput() on the
// child returns the same writer as the parent, without requiring a SetOutput call.
// l.charm is immutable after construction (no method replaces it), so it is safe
// to access without a lock.
func (l *AtmosLogger) WithPrefix(prefix string) *AtmosLogger {
	l.mu.RLock()
	w := l.writer
	l.mu.RUnlock()
	return &AtmosLogger{charm: l.charm.WithPrefix(prefix), writer: w}
}

// With returns a new logger with the given key-value pairs.
// The output writer is propagated to the child logger (see WithPrefix).
// l.charm is immutable after construction, so it is safe to access without a lock.
func (l *AtmosLogger) With(keyvals ...interface{}) *AtmosLogger {
	l.mu.RLock()
	w := l.writer
	l.mu.RUnlock()
	return &AtmosLogger{charm: l.charm.With(keyvals...), writer: w}
}

// GetLevelString returns the string representation of the current log level handling our custom levels appropriately.
func (l *AtmosLogger) GetLevelString() string {
	level := l.GetLevel()
	switch level {
	case TraceLevel:
		return "trace"
	case Level(math.MaxInt32):
		return "off"
	default:
		return level.String()
	}
}

// Helper marks the logger as a helper.
func (l *AtmosLogger) Helper() {
	l.charm.Helper()
}

// SetReportCaller sets whether to report the caller location.
func (l *AtmosLogger) SetReportCaller(report bool) {
	l.charm.SetReportCaller(report)
}

// SetReportTimestamp sets whether to report timestamps.
func (l *AtmosLogger) SetReportTimestamp(report bool) {
	l.charm.SetReportTimestamp(report)
}

// SetTimeFormat sets the time format.
func (l *AtmosLogger) SetTimeFormat(format string) {
	l.charm.SetTimeFormat(format)
}

// SetPrefix sets the logger prefix.
func (l *AtmosLogger) SetPrefix(prefix string) {
	l.charm.SetPrefix(prefix)
}

// GetPrefix returns the logger prefix.
func (l *AtmosLogger) GetPrefix() string {
	return l.charm.GetPrefix()
}

// Print logs a message at the info level.
func (l *AtmosLogger) Print(msg interface{}, keyvals ...interface{}) {
	l.charm.Print(msg, keyvals...)
}

// Printf logs a formatted message at the info level.
func (l *AtmosLogger) Printf(format string, args ...interface{}) {
	l.charm.Printf(format, args...)
}
