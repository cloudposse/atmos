package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// InitFlags returns the flag registry for the packer init command.
// Init uses the standard packer flags.
func InitFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "packer.InitFlags")()

	return flags.PackerFlags()
}

// InitPositionalArgs builds the positional args validator for packer init.
// Packer init requires: init <component>.
func InitPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "packer.InitPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
