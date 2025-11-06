package authexec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/flags"
)

// AuthExecOptions contains parsed flag values for auth exec command.
// This command passes through arguments to child processes.
type AuthExecOptions struct {
	flags.GlobalFlags

	// Identity is the authentication identity to use.
	Identity IdentityFlag

	// PositionalArgs contains non-flag arguments (the command to execute).
	PositionalArgs []string

	// SeparatedArgs contains args after -- separator.
	SeparatedArgs []string
}

// GetPositionalArgs returns the positional arguments.
func (o *AuthExecOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flags.AuthExecOptions.GetPositionalArgs")()

	return o.PositionalArgs
}

// GetSeparatedArgs returns the pass-through arguments.
func (o *AuthExecOptions) GetSeparatedArgs() []string {
	defer perf.Track(nil, "flags.AuthExecOptions.GetSeparatedArgs")()

	return o.SeparatedArgs
}

// AuthShellOptions contains parsed flag values for auth shell command.
// This command launches an interactive shell with authentication.
type AuthShellOptions struct {
	flags.GlobalFlags

	// Identity is the authentication identity to use.
	Identity IdentityFlag

	// Shell is the shell to use (defaults to $SHELL, then bash, then sh).
	Shell string

	// PositionalArgs contains non-flag arguments (shell arguments).
	PositionalArgs []string

	// SeparatedArgs contains args after -- separator.
	SeparatedArgs []string
}

// GetPositionalArgs returns the positional arguments.
func (o *AuthShellOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flags.AuthShellOptions.GetPositionalArgs")()

	return o.PositionalArgs
}

// GetSeparatedArgs returns the pass-through arguments.
func (o *AuthShellOptions) GetSeparatedArgs() []string {
	defer perf.Track(nil, "flags.AuthShellOptions.GetSeparatedArgs")()

	return o.SeparatedArgs
}

// IdentityFlag represents an identity flag value with helper methods.
type IdentityFlag struct {
	value string
}

// NewIdentityFlag creates a new IdentityFlag.
func NewIdentityFlag(value string) IdentityFlag {
	defer perf.Track(nil, "flags.NewIdentityFlag")()

	return IdentityFlag{value: value}
}

// Value returns the identity value.
func (i IdentityFlag) Value() string {
	defer perf.Track(nil, "flags.IdentityFlag.Value")()

	return i.value
}

// IsInteractiveSelector returns true if the identity value is the special selector value.
func (i IdentityFlag) IsInteractiveSelector() bool {
	defer perf.Track(nil, "flags.IdentityFlag.IsInteractiveSelector")()

	return i.value == cfg.IdentityFlagSelectValue
}

// IsEmpty returns true if no identity was specified.
func (i IdentityFlag) IsEmpty() bool {
	defer perf.Track(nil, "flags.IdentityFlag.IsEmpty")()

	return i.value == ""
}
