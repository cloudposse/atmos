package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// DestroyFlags returns the flag registry for the helmfile destroy command.
// Destroy uses the standard helmfile flags.
func DestroyFlags() *flags.FlagRegistry {
	return flags.HelmfileFlags()
}

// DestroyPositionalArgs builds the positional args validator for helmfile destroy.
// Helmfile destroy requires: destroy <component>
func DestroyPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
