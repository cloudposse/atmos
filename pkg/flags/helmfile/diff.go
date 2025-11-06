package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DiffFlags returns the flag registry for the helmfile diff command.
// Diff uses the standard helmfile flags.
func DiffFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "helmfile.DiffFlags")()

	return flags.HelmfileFlags()
}

// DiffPositionalArgs builds the positional args validator for helmfile diff.
// Helmfile diff requires: diff <component>.
func DiffPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "helmfile.DiffPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
