package flagparser

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthInterpreter provides strongly-typed access to auth command flags.
// Embeds GlobalFlags and adds auth-specific flags.
type AuthInterpreter struct {
	GlobalFlags // Embedded global flags

	// Identity specifies the target identity to assume.
	// Supports interactive selection via IdentitySelector.
	Identity IdentitySelector

	// Shell specifies the shell to launch (auth shell command only).
	// Empty string means use $SHELL or /bin/sh.
	Shell string

	// Command args (for auth exec).
	// Everything after flags that's not an Atmos flag.
	positionalArgs []string

	// Pass-through args for commands.
	// Everything after -- separator or non-Atmos flags.
	passThroughArgs []string
}

// GetGlobalFlags returns a pointer to the embedded GlobalFlags.
// Implements CommandInterpreter interface.
func (a *AuthInterpreter) GetGlobalFlags() *GlobalFlags {
	defer perf.Track(nil, "flagparser.AuthInterpreter.GetGlobalFlags")()

	return &a.GlobalFlags
}

// GetPositionalArgs returns positional arguments extracted by the parser.
// For auth exec: empty (all args are pass-through).
// For auth shell: empty (all args are pass-through).
func (a *AuthInterpreter) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.AuthInterpreter.GetPositionalArgs")()

	return a.positionalArgs
}

// GetPassThroughArgs returns arguments to pass through to the executed command or shell.
// For auth exec: command and its arguments.
// For auth shell: shell arguments.
func (a *AuthInterpreter) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.AuthInterpreter.GetPassThroughArgs")()

	return a.passThroughArgs
}
