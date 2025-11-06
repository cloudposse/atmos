package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateFlags returns the flag registry for the packer validate command.
// Validate uses the standard packer flags.
func ValidateFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "packer.ValidateFlags")()

	return flags.PackerFlags()
}

// ValidatePositionalArgs builds the positional args validator for packer validate.
// Packer validate requires: validate <component>.
func ValidatePositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "packer.ValidatePositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
