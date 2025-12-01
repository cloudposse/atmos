package logger

import (
	"os"
	"sync/atomic"

	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"

	"github.com/cloudposse/atmos/pkg/terminal/env"
)

// defaultLogger is the global default AtmosLogger instance stored atomically.
var defaultLogger atomic.Value

func init() {
	// Initialize with charm's default logger.
	charmLogger := charm.Default()

	// Best-effort NO_COLOR detection during early initialization.
	// This happens before flags are parsed or atmos.yaml is loaded,
	// so we can only check environment variables (NO_COLOR, CLICOLOR, CLICOLOR_FORCE, FORCE_COLOR).
	// Later, SetupLogger() will reconfigure with full context (flags + config).
	if colorEnabled := env.IsColorEnabled(); colorEnabled != nil && !*colorEnabled {
		charmLogger.SetColorProfile(termenv.Ascii)
	}

	defaultLogger.Store(NewAtmosLogger(charmLogger))
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
