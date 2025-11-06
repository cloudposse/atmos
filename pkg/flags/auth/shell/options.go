package shell

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
)

// AuthShellOptions contains parsed command-line options for auth shell command.
type AuthShellOptions struct {
	global.Flags
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
