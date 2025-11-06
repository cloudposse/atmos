package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// BuildFlags returns the flag registry for the packer build command.
// Build uses the standard packer flags.
func BuildFlags() *flags.FlagRegistry {
	return flags.PackerFlags()
}

// BuildPositionalArgs builds the positional args validator for packer build.
// Packer build requires: build <component>
func BuildPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
