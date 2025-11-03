package flags

import (
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthOptions provides strongly-typed access to auth command flags.
// Used for auth commands (auth console, auth exec, auth shell, auth validate, auth whoami, etc.).
//
// Embeds GlobalFlags for global Atmos flags (identity, chdir, config, logs, etc.).
// Provides auth-specific command fields (Verbose, Output, Console flags, etc.).
type AuthOptions struct {
	GlobalFlags // Embedded global flags (identity, chdir, config, logs, pager, profiling, etc.)

	// Common auth command flags.
	Verbose bool   // Enable verbose output (--verbose, -v)
	Output  string // Output format: table, json, yaml (--output, -o)

	// Console-specific flags (for auth console command).
	Destination      string        // Console page to navigate to (--destination)
	Duration         time.Duration // Console session duration (--duration)
	DurationProvided bool          // True if --duration was explicitly provided
	Issuer           string        // Issuer identifier for console session (--issuer)
	PrintOnly        bool          // Print URL to stdout without opening browser (--print-only)
	NoOpen           bool          // Don't open browser automatically (--no-open)
}

// Value returns the resolved auth options for use in command execution.
// This allows commands to access all options through a single object.
func (a *AuthOptions) Value() AuthOptions {
	defer perf.Track(nil, "flags.AuthOptions.Value")()

	return *a
}
