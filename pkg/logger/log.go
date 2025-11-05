package logger

import (
	"io"

	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// Package-level functions that delegate to the default global logger.
// These make the logger package a drop-in replacement for charm/log.

// Trace logs a trace message using the default logger.
func Trace(msg interface{}, keyvals ...interface{}) {
	Default().Trace(msg, keyvals...)
}

// Tracef logs a formatted trace message using the default logger.
func Tracef(format string, args ...interface{}) {
	Default().Tracef(format, args...)
}

// Debug logs a debug message using the default logger.
func Debug(msg interface{}, keyvals ...interface{}) {
	Default().Debug(msg, keyvals...)
}

// Debugf logs a formatted debug message using the default logger.
func Debugf(format string, args ...interface{}) {
	Default().Debugf(format, args...)
}

// Info logs an info message using the default logger.
func Info(msg interface{}, keyvals ...interface{}) {
	Default().Info(msg, keyvals...)
}

// Infof logs a formatted info message using the default logger.
func Infof(format string, args ...interface{}) {
	Default().Infof(format, args...)
}

// Warn logs a warning message using the default logger.
func Warn(msg interface{}, keyvals ...interface{}) {
	Default().Warn(msg, keyvals...)
}

// Warnf logs a formatted warning message using the default logger.
func Warnf(format string, args ...interface{}) {
	Default().Warnf(format, args...)
}

// Error logs an error message using the default logger.
func Error(msg interface{}, keyvals ...interface{}) {
	Default().Error(msg, keyvals...)
}

// Errorf logs a formatted error message using the default logger.
func Errorf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
}

// Fatal logs a fatal message using the default logger and exits.
func Fatal(msg interface{}, keyvals ...interface{}) {
	Default().Fatal(msg, keyvals...)
}

// Fatalf logs a formatted fatal message using the default logger and exits.
func Fatalf(format string, args ...interface{}) {
	Default().Fatalf(format, args...)
}

// Log logs a message at the given level using the default logger.
func Log(level charm.Level, msg interface{}, keyvals ...interface{}) {
	Default().Log(level, msg, keyvals...)
}

// Logf logs a formatted message at the given level using the default logger.
func Logf(level charm.Level, format string, args ...interface{}) {
	Default().Logf(level, format, args...)
}

// SetLevel sets the log level on the default logger.
func SetLevel(level charm.Level) {
	Default().SetLevel(level)
}

// GetLevel returns the current log level from the default logger.
func GetLevel() charm.Level {
	return Default().GetLevel()
}

// SetOutput sets the output writer on the default logger.
func SetOutput(w io.Writer) {
	Default().SetOutput(w)
}

// SetStyles sets the log styles on the default logger.
func SetStyles(styles *charm.Styles) {
	Default().SetStyles(styles)
}

// SetColorProfile sets the color profile on the default logger.
func SetColorProfile(profile termenv.Profile) {
	Default().SetColorProfile(profile)
}

// GetLevelString returns the string representation of the current log level.
func GetLevelString() string {
	return Default().GetLevelString()
}

// LevelToString converts a charm.Level to its string representation.
// It handles Atmos' custom TraceLevel and falls back to the standard string representation for other levels.
//
// Parameters:
//   - level: The charm.Level to convert
//
// Returns:
//   - string: The lowercase string representation of the level (e.g., "trace", "debug", "info", "warn", "error", "fatal")
//
// Examples:
//   - LevelToString(TraceLevel) returns "trace"
//   - LevelToString(charm.InfoLevel) returns "info"
//   - LevelToString(charm.ErrorLevel) returns "error"
func LevelToString(level charm.Level) string {
	switch level {
	case TraceLevel:
		return "trace"
	case charm.DebugLevel:
		return "debug"
	case charm.InfoLevel:
		return "info"
	case charm.WarnLevel:
		return "warn"
	case charm.ErrorLevel:
		return "error"
	case charm.FatalLevel:
		return "fatal"
	default:
		return level.String()
	}
}

// SetReportCaller sets whether to report the caller location on the default logger.
func SetReportCaller(report bool) {
	Default().SetReportCaller(report)
}

// SetReportTimestamp sets whether to report timestamps on the default logger.
func SetReportTimestamp(report bool) {
	Default().SetReportTimestamp(report)
}

// SetTimeFormat sets the time format on the default logger.
func SetTimeFormat(format string) {
	Default().SetTimeFormat(format)
}

// SetPrefix sets the logger prefix on the default logger.
func SetPrefix(prefix string) {
	Default().SetPrefix(prefix)
}

// GetPrefix returns the logger prefix from the default logger.
func GetPrefix() string {
	return Default().GetPrefix()
}

// With returns a new logger with the given key-value pairs.
func With(keyvals ...interface{}) *AtmosLogger {
	return Default().With(keyvals...)
}

// WithPrefix returns a new logger with the given prefix.
func WithPrefix(prefix string) *AtmosLogger {
	return Default().WithPrefix(prefix)
}

// Helper marks the default logger as a helper.
func Helper() {
	Default().Helper()
}

// Print logs a message at the info level using the default logger.
func Print(msg interface{}, keyvals ...interface{}) {
	Default().Print(msg, keyvals...)
}

// Printf logs a formatted message at the info level using the default logger.
func Printf(format string, args ...interface{}) {
	Default().Printf(format, args...)
}

// DefaultStyles returns the default charm log styles.
func DefaultStyles() *charm.Styles {
	return charm.DefaultStyles()
}

// Type exports for compatibility.
type (
	// Level is the log level type.
	Level = charm.Level
	// Styles is the log styles type.
	Styles = charm.Styles
)

// Level constants for compatibility.
const (
	// TraceLevel is the trace log level.
	TraceLevel = charm.DebugLevel - 1
	// DebugLevel is the debug log level.
	DebugLevel = charm.DebugLevel
	// InfoLevel is the info log level.
	InfoLevel = charm.InfoLevel
	// WarnLevel is the warning log level.
	WarnLevel = charm.WarnLevel
	// ErrorLevel is the error log level.
	ErrorLevel = charm.ErrorLevel
	// FatalLevel is the fatal log level.
	FatalLevel = charm.FatalLevel
)
