package store

import (
	"io"

	al "github.com/jfrog/jfrog-client-go/utils/log"
)

// noopLogger implements the jfrog-client-go/utils/log.Log interface
// to completely disable all logging from the JFrog SDK.
type noopLogger struct{}

// GetLogLevel returns the current log level.
func (l *noopLogger) GetLogLevel() al.LevelType {
	return al.ERROR // Just a placeholder, will be ignored
}

// SetLogLevel sets the log level.
// This implementation does nothing as we want to suppress all logging.
func (l *noopLogger) SetLogLevel(levelType al.LevelType) {
	// Do nothing
}

// SetOutputWriter sets the log output writer.
// This implementation ignores the writer since we want to suppress logging.
func (l *noopLogger) SetOutputWriter(writer io.Writer) {
	// Do nothing
}

// GetOutputWriter returns the log output writer.
// Always returns io.Discard to ensure no logs are written.
func (l *noopLogger) GetOutputWriter() io.Writer {
	return io.Discard // Discard all output
}

// SetLogsWriter sets the logs writer.
// This implementation ignores the writer since we want to suppress logging.
func (l *noopLogger) SetLogsWriter(writer io.Writer) {
	// Do nothing
}

// Debug logs a debug message.
// This implementation does nothing to suppress all logging.
func (l *noopLogger) Debug(a ...interface{}) {
	// Do nothing
}

// Info logs an info message.
// This implementation does nothing to suppress all logging.
func (l *noopLogger) Info(a ...interface{}) {
	// Do nothing
}

// Warn logs a warning message.
// This implementation does nothing to suppress all logging.
func (l *noopLogger) Warn(a ...interface{}) {
	// Do nothing
}

// Error logs an error message.
// This implementation does nothing to suppress all logging.
func (l *noopLogger) Error(a ...interface{}) {
	// Do nothing
}

// Output writes the output message.
// This implementation does nothing to suppress all logging.
func (l *noopLogger) Output(a ...interface{}) {
	// Do nothing
}

// createNoopLogger returns a new noopLogger instance that implements the al.Log interface.
// For testing purposes, this variable can be replaced to track noopLogger creation.
var createNoopLogger = func() al.Log {
	return &noopLogger{}
}
