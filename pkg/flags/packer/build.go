package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BuildFlags returns the flag registry for the packer build command.
// Build uses the standard packer flags.
func BuildFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "packer.BuildFlags")()

	return flags.PackerFlags()
}

// BuildPositionalArgs builds the positional args validator for packer build.
// Packer build requires: build <component>.
func BuildPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "packer.BuildPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
