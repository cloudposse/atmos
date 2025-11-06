package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// DiffFlags returns the flag registry for the helmfile diff command.
// Diff uses the standard helmfile flags.
func DiffFlags() *flags.FlagRegistry {
	return flags.HelmfileFlags()
}

// DiffPositionalArgs builds the positional args validator for helmfile diff.
// Helmfile diff requires: diff <component>
func DiffPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
