package packer

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// InitFlags returns the flag registry for the packer init command.
// Init uses the standard packer flags.
func InitFlags() *flags.FlagRegistry {
	return flags.PackerFlags()
}

// InitPositionalArgs builds the positional args validator for packer init.
// Packer init requires: init <component>
func InitPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
