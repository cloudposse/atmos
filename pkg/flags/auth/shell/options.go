package shell

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "shell.AuthShellOptions.GetPositionalArgs")()

	return o.PositionalArgs
}

// GetSeparatedArgs returns the separated (pass-through) arguments.
func (o *AuthShellOptions) GetSeparatedArgs() []string {
	defer perf.Track(nil, "shell.AuthShellOptions.GetSeparatedArgs")()

	return o.SeparatedArgs
}
