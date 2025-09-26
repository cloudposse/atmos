package logger

import (
	"os"
	"sync/atomic"

	charm "github.com/charmbracelet/log"
)

// defaultLogger is the global default AtmosLogger instance stored atomically.
var defaultLogger atomic.Value

func init() {
	// Initialize with charm's default logger.
	defaultLogger.Store(NewAtmosLogger(charm.Default()))
}

// Default returns the global default AtmosLogger instance.
func Default() *AtmosLogger {
	return defaultLogger.Load().(*AtmosLogger)
}

// SetDefault sets a new global default AtmosLogger instance.
func SetDefault(logger *AtmosLogger) {
	if logger != nil {
		defaultLogger.Store(logger)
	}
}

// New creates a new AtmosLogger with default settings.
func New() *AtmosLogger {
	return NewAtmosLogger(charm.New(os.Stderr))
}
