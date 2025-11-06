package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DestroyFlags returns the flag registry for the helmfile destroy command.
// Destroy uses the standard helmfile flags.
func DestroyFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "helmfile.DestroyFlags")()

	return flags.HelmfileFlags()
}

// DestroyPositionalArgs builds the positional args validator for helmfile destroy.
// Helmfile destroy requires: destroy <component>.
func DestroyPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "helmfile.DestroyPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
