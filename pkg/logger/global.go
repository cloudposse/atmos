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
	// Initialize with charm's default logger options (timestamps on), bound to stderr
	// through a writer that hides the file descriptor. charm.Default() would build a
	// color-caching renderer on os.Stderr, which queries the terminal (OSC 10/11) and
	// blocks for seconds when the terminal never answers.
	charmLogger := newCharmLogger(os.Stderr, true)

	// Install it as charm's default logger so that code calling charm's package-level
	// functions (log.Info, log.Warn, ...) shares this logger's level, styles, and output,
	// and so charm never lazily creates its own (querying) default logger.
	charm.SetDefault(charmLogger)

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
	return NewAtmosLogger(newCharmLogger(os.Stderr, false))
}
