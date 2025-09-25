package logger

import (
	"math"

	log "github.com/charmbracelet/log"
)

// TraceLevel is one level more verbose than Debug.
// This extends Charm Bracelet log with trace support.
const TraceLevel = log.DebugLevel - 1

// Trace logs a trace message using the default logger.
func Trace(msg interface{}, keyvals ...interface{}) {
	log.Log(TraceLevel, msg, keyvals...)
}

// Tracef logs a formatted trace message using the default logger.
func Tracef(format string, args ...interface{}) {
	log.Logf(TraceLevel, format, args...)
}

// GetLevelString returns the string representation of the current log level,
// handling our custom levels appropriately.
func GetLevelString() string {
	level := log.GetLevel()
	switch level {
	case TraceLevel:
		return "trace"
	case log.Level(math.MaxInt32):
		return "off"
	default:
		return level.String()
	}
}
