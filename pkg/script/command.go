package script

import (
	"context"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandSpec describes a standalone program's command-line interface using the
// same flag and positional argument definitions as native Atmos commands.
type CommandSpec struct {
	Name, Description string
	Args              []*flags.PositionalArgSpec
	Flags             []flags.Flag
}

// CommandInput contains validated inputs, or Help when only usage was requested.
// Optional positional arguments that were omitted have a nil value.
type CommandInput struct {
	Args, Flags map[string]any
	Help        bool
	// Usage turns a rejection of the parsed input, such as a failed validate callback, into the
	// host's usage error: exit status 2, the usage line, and a --help hint. Nil when the host has
	// no such presentation.
	Usage func(cause error) error
}

// UsageFailure carries a host-presented usage error through a language runtime unchanged. The user
// mistyped the command line; the script did not fail, so the runtime reports Err as-is, without a
// language prefix or a traceback.
type UsageFailure struct{ Err error }

// Error returns the usage message.
func (u *UsageFailure) Error() string {
	defer perf.Track(nil, "script.UsageFailure.Error")()

	return u.Err.Error()
}

// Unwrap exposes the usage error so errors.Is sees ErrScriptUsage.
func (u *UsageFailure) Unwrap() error {
	defer perf.Track(nil, "script.UsageFailure.Unwrap")()

	return u.Err
}

// CommandParser is the host boundary for standalone command parsing and help.
// The language runtime calls user functions only after parsing succeeds.
type CommandParser func(context.Context, CommandSpec) (CommandInput, error)
