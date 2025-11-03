package flags

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// AuthExecOptions contains parsed flag values for auth exec command.
// This command passes through arguments to child processes.
type AuthExecOptions struct {
	GlobalFlags

	// Identity is the authentication identity to use.
	Identity IdentityFlag

	// PositionalArgs contains non-flag arguments (the command to execute).
	PositionalArgs []string

	// PassThroughArgs contains args after -- separator.
	PassThroughArgs []string
}

// GetPositionalArgs returns the positional arguments.
func (o *AuthExecOptions) GetPositionalArgs() []string {
	return o.PositionalArgs
}

// GetPassThroughArgs returns the pass-through arguments.
func (o *AuthExecOptions) GetPassThroughArgs() []string {
	return o.PassThroughArgs
}

// AuthShellOptions contains parsed flag values for auth shell command.
// This command launches an interactive shell with authentication.
type AuthShellOptions struct {
	GlobalFlags

	// Identity is the authentication identity to use.
	Identity IdentityFlag

	// Shell is the shell to use (defaults to $SHELL, then bash, then sh).
	Shell string

	// PositionalArgs contains non-flag arguments (shell arguments).
	PositionalArgs []string

	// PassThroughArgs contains args after -- separator.
	PassThroughArgs []string
}

// GetPositionalArgs returns the positional arguments.
func (o *AuthShellOptions) GetPositionalArgs() []string {
	return o.PositionalArgs
}

// GetPassThroughArgs returns the pass-through arguments.
func (o *AuthShellOptions) GetPassThroughArgs() []string {
	return o.PassThroughArgs
}

// IdentityFlag represents an identity flag value with helper methods.
type IdentityFlag struct {
	value string
}

// NewIdentityFlag creates a new IdentityFlag.
func NewIdentityFlag(value string) IdentityFlag {
	return IdentityFlag{value: value}
}

// Value returns the identity value.
func (i IdentityFlag) Value() string {
	return i.value
}

// IsInteractiveSelector returns true if the identity value is the special selector value.
func (i IdentityFlag) IsInteractiveSelector() bool {
	return i.value == cfg.IdentityFlagSelectValue
}

// IsEmpty returns true if no identity was specified.
func (i IdentityFlag) IsEmpty() bool {
	return i.value == ""
}
