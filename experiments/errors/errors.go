//go:build !linting
// +build !linting

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
)

// Define TRACE log level (one step below DEBUG).
const TraceLevel log.Level = log.DebugLevel - 1

// AtmosError represents a structured error.
type AtmosError struct {
	Base     error
	Message  string
	Meta     map[string]any
	Tips     []string
	ExitCode int
}

// Error implements the error interface.
func (e *AtmosError) Error() string {
	return e.Message
}

// Unwrap allows errors.Is and errors.As to work with AtmosError.
func (e *AtmosError) Unwrap() error {
	return e.Base
}

// Fields implements log.Fielder but excludes tips and exit code.
func (e *AtmosError) Fields() map[string]any {
	fields := make(map[string]any)

	// Copy metadata fields
	for k, v := range e.Meta {
		fields[k] = v
	}

	return fields
}

// WithContext adds metadata.
func (e *AtmosError) WithContext(keyvals ...any) *AtmosError {
	for i := 0; i < len(keyvals)-1; i += 2 {
		if key, ok := keyvals[i].(string); ok {
			e.Meta[key] = keyvals[i+1]
		}
	}

	return e
}

// WithTips adds helpful suggestions.
func (e *AtmosError) WithTips(tips ...string) *AtmosError {
	e.Tips = append(e.Tips, tips...)
	return e
}

// WithExitCode sets the exit code.
func (e *AtmosError) WithExitCode(code int) *AtmosError {
	e.ExitCode = code
	return e
}

// NewBaseError creates reusable errors.
func NewBaseError(message string) *AtmosError {
	return &AtmosError{
		Base:     errors.New(message),
		Message:  message,
		Meta:     make(map[string]any),
		ExitCode: 1, // Default non-zero exit code
	}
}

// NewAtmosError creates a structured error dynamically.
func NewAtmosError(message string, keyvals ...any) *AtmosError {
	return &AtmosError{
		Base:    errors.New(message),
		Message: message,
		Meta:    parseKeyVals(keyvals),
	}
}

// parseKeyVals converts variadic key-value pairs into a map.
func parseKeyVals(keyvals []any) map[string]any {
	fields := make(map[string]any)

	for i := 0; i < len(keyvals)-1; i += 2 {
		if key, ok := keyvals[i].(string); ok {
			fields[key] = keyvals[i+1]
		}
	}

	return fields
}

// AtmosLogger wraps log.Logger.
type AtmosLogger struct {
	*log.Logger
}

func PrintTips(tips []string) {
	if len(tips) == 0 {
		return // Avoid unnecessary writes
	}

	for _, tip := range tips {
		fmt.Fprintln(os.Stderr, "  💡", tip) // Print to stderr for consistency
	}

	// Ensure the output is flushed immediately
	os.Stderr.Sync()
}

func flattenFields(fields map[string]any) []any {
	var keyvals []any
	for k, v := range fields {
		keyvals = append(keyvals, k, v)
	}

	return keyvals
}

// Error logs a message at ERROR level.
func (l *AtmosLogger) Error(msgOrErr any, keyvals ...any) {
	l.Log(log.ErrorLevel, msgOrErr, keyvals...)
}

// Warn logs a message at WARN level.
func (l *AtmosLogger) Warn(msgOrErr any, keyvals ...any) {
	l.Log(log.WarnLevel, msgOrErr, keyvals...)
}

// Info logs a message at INFO level.
func (l *AtmosLogger) Info(msgOrErr any, keyvals ...any) {
	l.Log(log.InfoLevel, msgOrErr, keyvals...)
}

// Debug logs a message at DEBUG level.
func (l *AtmosLogger) Debug(msgOrErr any, keyvals ...any) {
	l.Log(log.DebugLevel, msgOrErr, keyvals...)
}

// Trace logs a message at TRACE level.
func (l *AtmosLogger) Trace(msgOrErr any, keyvals ...any) {
	l.Log(TraceLevel, msgOrErr, keyvals...)
}

func (l *AtmosLogger) Fatal(msgOrErr any, keyvals ...any) {
	l.Log(log.FatalLevel, msgOrErr, keyvals...)

	if e, ok := msgOrErr.(*AtmosError); ok {
		os.Exit(e.ExitCodeOrDefault())
	}
}

func (l *AtmosLogger) Log(level log.Level, msgOrErr any, keyvals ...any) {
	var (
		message string
		tips    []string
	)

	switch v := msgOrErr.(type) {
	case *AtmosError:
		message = v.Message
		tips = v.Tips
		// Append metadata fields from the AtmosError
		keyvals = append(flattenFields(v.Fields()), keyvals...)
	case error:
		message = v.Error()
	case string:
		message = v
	default:
		message = "Unknown error format"
	}

	// Log message at the given level
	l.Logger.Log(level, message, keyvals...) // Ensure keyvals are correctly expanded

	// Always print tips if available
	PrintTips(tips)
}

func (e *AtmosError) ExitCodeOrDefault() int {
	if e.ExitCode != 0 {
		return e.ExitCode
	}

	return 1
}
