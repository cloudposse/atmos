package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SyncFlags returns the flag registry for the helmfile sync command.
// Sync uses the standard helmfile flags.
func SyncFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "helmfile.SyncFlags")()

	return flags.HelmfileFlags()
}

// SyncPositionalArgs builds the positional args validator for helmfile sync.
// Helmfile sync requires: sync <component>.
func SyncPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "helmfile.SyncPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
