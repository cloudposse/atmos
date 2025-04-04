package logger

import (
	"fmt"
	"io"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
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

// Error logs at ERROR level with different input types, supporting structured logging.
// It accepts:
// - String message: Error("event_name", "key", "value").
// - Error object: Error(err, "key", "value").
// - AtmosError: Error(atmosErr, "key", "value"). // atmosErr is of type *errors.AtmosError.
func (l *Logger) Error(errOrMsg interface{}, keyvals ...interface{}) {
	// Only proceed if the log level allows for error messages
	if l.LogLevel == LogLevelOff {
		return
	}

	// Use AtmosLogger to handle the structured error logging
	atmosLogger := &AtmosLogger{Logger: log.New(os.Stderr)}
	atmosLogger.Error(errOrMsg, keyvals...)
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

// processLogMessage extracts the appropriate message and processes keyvals based on msgOrErr type.
func processLogMessage(msgOrErr interface{}, keyvals []interface{}) (string, []interface{}) {
	var msg string

	switch v := msgOrErr.(type) {
	case *errors.AtmosError:
		msg = v.Message

		if v.Base != nil {
			keyvals = append([]interface{}{"base_error", v.Base.Error()}, keyvals...)
		}

		if len(v.Meta) > 0 {
			for k, val := range v.Meta {
				keyvals = append(keyvals, k, val)
			}
		}

	case error:
		msg = v.Error()
		keyvals = append([]interface{}{"error", v.Error()}, keyvals...)

	case string:
		msg = v

	default:
		msg = fmt.Sprintf("%v", v)
	}

	return msg, keyvals
}

// validateKeyValuePairs ensures keyvals has an even number of elements.
func validateKeyValuePairs(l *AtmosLogger, keyvals []interface{}) []interface{} {
	if len(keyvals)%2 != 0 {
		oddIndex := len(keyvals) - 1
		oddKey := fmt.Sprintf("%v", keyvals[oddIndex])

		l.Logger.Error("odd number of key-value pairs",
			"key_without_value", oddKey)

		keyvals = keyvals[:oddIndex]
	}

	return keyvals
}

// handleTips displays tips for appropriate error levels.
func handleTips(level log.Level, msgOrErr interface{}) {
	if atmosErr, ok := msgOrErr.(*errors.AtmosError); ok {
		if level >= log.ErrorLevel && len(atmosErr.Tips) > 0 {
			PrintTips(atmosErr.Tips)
		}
	}
}

// Log implements the core logging functionality with intelligent handling of different input types.
func (l *AtmosLogger) Log(level log.Level, msgOrErr interface{}, keyvals ...interface{}) {
	// Process the message based on type
	msg, processedKeyvals := processLogMessage(msgOrErr, keyvals)

	// Validate key-value pairs
	processedKeyvals = validateKeyValuePairs(l, processedKeyvals)

	// Log the message
	l.Logger.Log(level, msg, processedKeyvals...)

	// Handle tips for display
	handleTips(level, msgOrErr)
}

// Error logs at ERROR level with different input types.
// It accepts:
// - String message: Error("event_name", "key", "value").
// - Error object: Error(err, "key", "value").
// - AtmosError: Error(atmosErr, "key", "value"). // atmosErr is of type *errors.AtmosError.
func (l *AtmosLogger) Error(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.ErrorLevel, msgOrErr, keyvals...)
}

// Warning logs at WARN level with different input types.
// It accepts:
// - String message: Warning("event_name", "key", "value").
// - Error object: Warning(err, "key", "value").
// - AtmosError: Warning(atmosErr, "key", "value"). // atmosErr is of type *errors.AtmosError.
func (l *AtmosLogger) Warning(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.WarnLevel, msgOrErr, keyvals...)
}

// Info logs at INFO level with different input types.
// It accepts:
// - String message: Info("event_name", "key", "value").
// - Error object: Info(err, "key", "value").
// - AtmosError: Info(atmosErr, "key", "value"). // atmosErr is of type *errors.AtmosError.
func (l *AtmosLogger) Info(msgOrErr interface{}, keyvals ...interface{}) {
	l.Log(log.InfoLevel, msgOrErr, keyvals...)
}

// Debug logs at DEBUG level with different input types.
// It accepts:
// - String message: Debug("event_name", "key", "value").
// - Error object: Debug(err, "key", "value").
// - AtmosError: Debug(atmosErr, "key", "value"). // atmosErr is of type *errors.AtmosError.
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

// PrintTips displays user guidance tips in a console-friendly format.
func PrintTips(tips []string) {
	if len(tips) == 0 {
		return
	}

	for i, tip := range tips {
		u.PrintfMessageToTUI("  %s %s\n",
			theme.Colors.Info.Sprintf("%d.", i+1),
			tip)
	}
}
