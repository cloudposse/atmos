package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ApplyFlags returns the flag registry for the helmfile apply command.
// Apply uses the standard helmfile flags.
func ApplyFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "helmfile.ApplyFlags")()

	return flags.HelmfileFlags()
}

// ApplyPositionalArgs builds the positional args validator for helmfile apply.
// Helmfile apply requires: apply <component>.
func ApplyPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "helmfile.ApplyPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
