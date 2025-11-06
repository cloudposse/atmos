package authshell

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// AuthShellOptions contains parsed command-line options for auth shell command.
type AuthShellOptions struct {
	flags.GlobalFlags
	Identity       flags.IdentitySelector
	Shell          string
	PositionalArgs []string
	SeparatedArgs  []string
}

// GetPositionalArgs returns the positional arguments.
func (o *AuthShellOptions) GetPositionalArgs() []string {
	return o.PositionalArgs
}

// GetSeparatedArgs returns the separated (pass-through) arguments.
func (o *AuthShellOptions) GetSeparatedArgs() []string {
	return o.SeparatedArgs
}
