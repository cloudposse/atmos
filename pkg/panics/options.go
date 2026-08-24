package panics

import (
	"io"
	"os"
	"strings"
	"time"
)

// writer is the minimal io.Writer-like contract used by the fallback
// output path. Defined here so tests can inject a bytes.Buffer without
// importing pkg/panics's internal state.
type writer interface {
	Write(p []byte) (int, error)
}

// Options controls HandlePanic behavior. Production callers use
// defaultOptions(); tests can pass a populated Options directly.
type Options struct {
	// crashDir is the directory where the crash report is written.
	// Defaults to os.TempDir().
	crashDir string
	// args is the command-line invocation recorded in the report.
	// Defaults to os.Args.
	args []string
	// now returns the current time; override for deterministic tests.
	now func() time.Time
	// exitCode is returned by HandlePanic. Defaults to PanicExitCode.
	exitCode int
	// showStackInline is true when the user asked for the stack in
	// the terminal (ATMOS_LOGS_LEVEL=Debug or Trace, or explicit test
	// override). --logs-level is intentionally NOT consulted: the
	// panic may fire before Cobra/Viper finish parsing flags, so
	// honoring only the env var keeps the contract predictable.
	showStackInline bool
	// useUI controls whether output goes through pkg/ui (true) or
	// directly to stderr (false). Tests set useUI=false and inject
	// a buffer via stderr.
	useUI bool
	// stderr is the fallback writer used when useUI is false. Tests
	// inject a bytes.Buffer; production leaves this nil because
	// useUI is true.
	stderr writer
}

// applyDefaults fills in any zero fields with production defaults.
// Called from HandlePanic so tests passing a partial Options still get
// sensible behavior.
func (o *Options) applyDefaults() {
	if o.crashDir == "" {
		o.crashDir = os.TempDir()
	}
	if o.args == nil {
		o.args = append([]string{}, os.Args...)
	}
	if o.now == nil {
		o.now = time.Now
	}
	if o.exitCode == 0 {
		o.exitCode = PanicExitCode
	}
}

// defaultOptions returns the production Options: ui-rendered output,
// stack gated on ATMOS_LOGS_LEVEL, crash report under os.TempDir(),
// args from os.Args.
func defaultOptions() Options {
	return Options{
		useUI:           true,
		showStackInline: stackInlineFromEnv(),
		stderr:          os.Stderr, // unused when useUI is true; safe fallback if ui isn't ready.
	}
}

// stackInlineFromEnv returns true when the environment indicates the
// user wants the raw stack in the terminal. We read the env var
// directly rather than going through viper — when the panic handler
// runs, the logger/config may or may not have been initialized, but
// env vars are always readable. This is a deliberate exception to
// the forbidigo rule banning os.Getenv; viper is unsafe here because
// the panic might fire before viper init.
//
// The canonical Atmos log levels are Title-cased ("Debug", "Trace"),
// but we match case-insensitively so users who type "debug" or
// "DEBUG" at the shell still get the expected behavior.
func stackInlineFromEnv() bool {
	level := strings.ToLower(strings.TrimSpace(os.Getenv(LogLevelEnvVar))) //nolint:forbidigo // panic handler may run before viper init.
	return level == "debug" || level == "trace"
}

// LogLevelEnvVar is the Atmos log-level environment variable. Callers
// can reference this constant instead of hard-coding the string.
const LogLevelEnvVar = "ATMOS_LOGS_LEVEL"

// Compile-time assertion that any io.Writer (io.Discard here) is
// assignable to the local writer interface. Keeps the two contracts
// aligned if someone narrows `writer`'s signature in the future.
var _ writer = io.Discard
