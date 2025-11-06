package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// ValidateFlags returns the flag registry for the packer validate command.
// Validate uses the standard packer flags.
func ValidateFlags() *flags.FlagRegistry {
	return flags.PackerFlags()
}

// ValidatePositionalArgs builds the positional args validator for packer validate.
// Packer validate requires: validate <component>
func ValidatePositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
