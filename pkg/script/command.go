package script

import (
	"context"

	"github.com/cloudposse/atmos/pkg/flags"
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
}

// CommandParser is the host boundary for standalone command parsing and help.
// The language runtime calls user functions only after parsing succeeds.
type CommandParser func(context.Context, CommandSpec) (CommandInput, error)
