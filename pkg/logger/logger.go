package logger

import (
	"fmt"
	"io"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// LogLevel represents the verbosity level for logging.
type LogLevel string

// Log level constants from most verbose to least verbose.
const (
	LogLevelTrace   LogLevel = "Trace"
	LogLevelDebug   LogLevel = "Debug"
	LogLevelInfo    LogLevel = "Info"
	LogLevelWarning LogLevel = "Warning"
	LogLevelError   LogLevel = "Error"
	LogLevelOff     LogLevel = "Off"
)

// logLevelOrder defines the order of log levels from most verbose to least verbose.
var logLevelOrder = map[LogLevel]int{
	LogLevelTrace:   0,
	LogLevelDebug:   1,
	LogLevelInfo:    2,
	LogLevelWarning: 3,
	LogLevelError:   4,
	LogLevelOff:     5,
}

// AtmosError represents a structured error with rich context.
type AtmosError struct {
	Base     error                  // Underlying error for unwrapping
	Message  string                 // Human-readable message
	Meta     map[string]interface{} // Contextual metadata
	Tips     []string               // User guidance
	ExitCode int                    // Process exit code
}

// Error implements the error interface.
func (e *AtmosError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error.
func (e *AtmosError) Unwrap() error {
	return e.Base
}

// Fields returns the metadata fields for logging.
func (e *AtmosError) Fields() map[string]interface{} {
	fields := make(map[string]interface{})
	for k, v := range e.Meta {
		fields[k] = v
	}
	return fields
}

// WithContext adds context key-value pairs to the error metadata.
func (e *AtmosError) WithContext(keyvals ...interface{}) *AtmosError {
	if len(keyvals)%2 != 0 {
		// If odd number of arguments, add empty string to make it even
		keyvals = append(keyvals, "")
	}

	if e.Meta == nil {
		e.Meta = make(map[string]interface{})
	}

	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyvals[i])
		}
		e.Meta[key] = keyvals[i+1]
	}

	return e
}

// WithTips adds user guidance tips to the error.
func (e *AtmosError) WithTips(tips ...string) *AtmosError {
	e.Tips = append(e.Tips, tips...)
	return e
}

// NewAtmosError creates a new structured error with the given message and base error.
func NewAtmosError(message string, baseErr error) *AtmosError {
	return &AtmosError{
		Message: message,
		Base:    baseErr,
		Meta:    make(map[string]interface{}),
	}
}

// NewBaseError creates a new base error that can be used for error checking.
func NewBaseError(message string) *AtmosError {
	return NewAtmosError(message, nil)
}

// Logger implements a basic logging interface.
type Logger struct {
	LogLevel LogLevel
	File     string
}

func NewLogger(logLevel LogLevel, file string) (*Logger, error) {
	return &Logger{
		LogLevel: logLevel,
		File:     file,
	}, nil
}

func NewLoggerFromCliConfig(cfg schema.AtmosConfiguration) (*Logger, error) {
	logLevel, err := ParseLogLevel(cfg.Logs.Level)
	if err != nil {
		return nil, err
	}
	return NewLogger(logLevel, cfg.Logs.File)
}

// ParseLogLevel converts a string log level to the LogLevel type.
// Returns LogLevelInfo as default for empty strings.
func ParseLogLevel(logLevel string) (LogLevel, error) {
	if logLevel == "" {
		return LogLevelInfo, nil
	}

	validLevels := []LogLevel{LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelError, LogLevelOff}
	for _, level := range validLevels {
		if LogLevel(logLevel) == level {
			return level, nil
		}
	}

	return "", fmt.Errorf("Invalid log level `%s`. Valid options are: %v", logLevel, validLevels)
}

func (l *Logger) log(logColor *color.Color, message string) {
	if l.File != "" {
		if l.File == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else if l.File == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				color.Red("%s\n", err)
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					color.Red("%s\n", err)
				}
			}(f)

			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				color.Red("%s\n", err)
			}
		}
	} else {
		_, err := logColor.Fprintln(os.Stdout, message)
		if err != nil {
			color.Red("%s\n", err)
		}
	}
}

// SetLogLevel sets the logger's minimum log level.
func (l *Logger) SetLogLevel(logLevel LogLevel) error {
	l.LogLevel = logLevel
	return nil
}

// isLevelEnabled checks if a given log level should be enabled based on the logger's current level.
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	if l.LogLevel == LogLevelOff {
		return false
	}
	return logLevelOrder[level] >= logLevelOrder[l.LogLevel]
}

// Error logs an error message if the error is not nil and the log level is not Off.
func (l *Logger) Error(err error) {
	if err != nil && l.LogLevel != LogLevelOff {
		_, err2 := theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}
	}
}

// Trace logs a message at TRACE level if the current log level permits.
func (l *Logger) Trace(message string) {
	if l.isLevelEnabled(LogLevelTrace) {
		l.log(theme.Colors.Info, message)
	}
}

// Debug logs a message at DEBUG level if the current log level permits.
func (l *Logger) Debug(message string) {
	if l.isLevelEnabled(LogLevelDebug) {
		l.log(theme.Colors.Info, message)
	}
}

// Info logs a message at INFO level if the current log level permits.
func (l *Logger) Info(message string) {
	if l.isLevelEnabled(LogLevelInfo) {
		l.log(theme.Colors.Info, message)
	}
}

// Warning logs a message at WARNING level if the current log level permits.
func (l *Logger) Warning(message string) {
	if l.isLevelEnabled(LogLevelWarning) {
		l.log(theme.Colors.Warning, message)
	}
}

// Map Atmos log levels to Charmbracelet log levels for direct conversion.
var atmosToCharmLevelMap = map[string]log.Level{
	string(LogLevelTrace):   log.DebugLevel, // TODO: Add trace level using charm custom level
	string(LogLevelDebug):   log.DebugLevel,
	string(LogLevelInfo):    log.InfoLevel,
	string(LogLevelWarning): log.WarnLevel,
	string(LogLevelError):   log.ErrorLevel,
	string(LogLevelOff):     log.FatalLevel + 1, // Higher than any level
}

// AtmosLogger wraps the Charmbracelet logger to provide enhanced structured semantic logging.
type AtmosLogger struct {
	*log.Logger
}

// NewAtmosLogger creates a new AtmosLogger with the given options.
func NewAtmosLogger(out io.Writer, options *log.Options) *AtmosLogger {
	return &AtmosLogger{
		Logger: log.NewWithOptions(out, *options),
	}
}

// NewDefaultAtmosLogger creates a new AtmosLogger with default settings.
func NewDefaultAtmosLogger() *AtmosLogger {
	return &AtmosLogger{
		Logger: log.Default(),
	}
}

// Log implements the core logging functionality with intelligent handling of different input types.
func (l *AtmosLogger) Log(level log.Level, msgOrErr interface{}, keyvals ...interface{}) {
	var msg string

	switch v := msgOrErr.(type) {
	case *AtmosError:
		msg = v.Message

		if v.Base != nil {
			keyvals = append([]interface{}{"base_error", v.Base.Error()}, keyvals...)
		}

		if len(v.Meta) > 0 {
			for k, val := range v.Meta {
				keyvals = append(keyvals, k, val)
			}
		}

		if len(v.Tips) > 0 {
			keyvals = append(keyvals, "tips", v.Tips)
		}

	case error:
		msg = v.Error()
		keyvals = append([]interface{}{"error", v.Error()}, keyvals...)

	case string:
		msg = v

	default:
		msg = fmt.Sprintf("%v", v)
	}

	// This is basically for the case where we have an odd number of key-value pairs
	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "")
	}

	l.Logger.Log(level, msg, keyvals...)
}

// Error logs at ERROR level with different input types.
// It accepts:
// - String message: Error("event_name", "key", "value").
// - Error object: Error(err, "key", "value").
// - AtmosError: Error(atmosErr, "key", "value").
func (l *AtmosLogger) Error(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.ErrorLevel, msgOrErr, keyvals...)
}

// Warning logs at WARN level with different input types.
// It accepts:
// - String message: Warning("event_name", "key", "value").
// - Error object: Warning(err, "key", "value").
// - AtmosError: Warning(atmosErr, "key", "value").
func (l *AtmosLogger) Warning(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.WarnLevel, msgOrErr, keyvals...)
}

// Info logs at INFO level with different input types.
// It accepts:
// - String message: Info("event_name", "key", "value").
// - Error object: Info(err, "key", "value").
// - AtmosError: Info(atmosErr, "key", "value").
func (l *AtmosLogger) Info(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.InfoLevel, msgOrErr, keyvals...)
}

// Debug logs at DEBUG level with different input types.
// It accepts:
// - String message: Debug("event_name", "key", "value").
// - Error object: Debug(err, "key", "value").
// - AtmosError: Debug(atmosErr, "key", "value").
func (l *AtmosLogger) Debug(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.DebugLevel, msgOrErr, keyvals...)
}

// Note: Charmbracelet logger doesn't have a TraceLevel, so we need to implement a custom method in another PR.

// SetAtmosLogLevel configures the logger level based on Atmos log level strings.
func SetAtmosLogLevel(l *AtmosLogger, level string) error {
	_, err := ParseLogLevel(level)
	if err != nil {
		return err
	}

	charmLevel, exists := atmosToCharmLevelMap[level]
	if exists {
		l.SetLevel(charmLevel)
	}
	return nil
}
