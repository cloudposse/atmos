package logger

import (
	"os"

	charm "github.com/charmbracelet/log"
)

// defaultLogger is the global default AtmosLogger instance.
var defaultLogger *AtmosLogger

func init() {
	// Initialize with charm's default logger.
	defaultLogger = NewAtmosLogger(charm.Default())
}

// Default returns the global default AtmosLogger instance.
func Default() *AtmosLogger {
	return defaultLogger
}

// SetDefault sets a new global default AtmosLogger instance.
func SetDefault(logger *AtmosLogger) {
	if logger != nil {
		defaultLogger = logger
	}
}

// New creates a new AtmosLogger with default settings.
func New() *AtmosLogger {
	return NewAtmosLogger(charm.New(os.Stderr))
}
