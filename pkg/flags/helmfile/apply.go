package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// ApplyFlags returns the flag registry for the helmfile apply command.
// Apply uses the standard helmfile flags.
func ApplyFlags() *flags.FlagRegistry {
	return flags.HelmfileFlags()
}

// ApplyPositionalArgs builds the positional args validator for helmfile apply.
// Helmfile apply requires: apply <component>
func ApplyPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
