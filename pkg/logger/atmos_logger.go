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
// Pass the same io.Writer that was used to construct charmLogger so that GetOutput()
// returns the correct writer immediately after construction. Pass nil to use os.Stderr,
// which matches the default writer of charm.Default() and charm.New(os.Stderr).
//
// If charmLogger was initialized with a non-stderr writer (e.g., charm.New(&buf)),
// pass that same writer so that GetOutput() returns the correct value immediately
// without requiring a follow-up SetOutput call.
func NewAtmosLogger(charmLogger *charm.Logger, w io.Writer) *AtmosLogger {
	if charmLogger == nil {
		charmLogger = charm.Default()
	}
	if w == nil {
		w = os.Stderr
	}
	return &AtmosLogger{charm: charmLogger, writer: w}
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
//
// Passing nil is treated as "reset to os.Stderr": both the tracked writer and
// charm's output are set to os.Stderr.
func (l *AtmosLogger) SetOutput(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	l.mu.Lock()
	l.writer = w
	l.charm.SetOutput(w)
	l.mu.Unlock()
}

// GetOutput returns the current output writer.
// If the writer has not been set (nil), os.Stderr is returned as the safe default.
func (l *AtmosLogger) GetOutput() io.Writer {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.writer == nil {
		return os.Stderr
	}
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
// The read lock is held for the entire duration of the charm call so that no
// concurrent SetOutput can update charm's writer between reading l.writer and
// calling l.charm.WithPrefix(prefix). l.charm is immutable after construction
// (no method replaces the pointer), so the read lock is sufficient.
//
// Writer snapshot: the returned logger captures the parent's output writer at
// the time of this call. Subsequent SetOutput calls on the parent do not
// propagate to the child.
func (l *AtmosLogger) WithPrefix(prefix string) *AtmosLogger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	child := l.charm.WithPrefix(prefix)
	return &AtmosLogger{charm: child, writer: l.writer}
}

// With returns a new logger with the given key-value pairs.
// The read lock is held for the entire duration of the charm call (see WithPrefix).
//
// Writer snapshot: the returned logger captures the parent's output writer at
// the time of this call. Subsequent SetOutput calls on the parent do not
// propagate to the child.
func (l *AtmosLogger) With(keyvals ...interface{}) *AtmosLogger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	child := l.charm.With(keyvals...)
	return &AtmosLogger{charm: child, writer: l.writer}
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
