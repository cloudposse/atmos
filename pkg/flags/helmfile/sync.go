package helmfile

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// SyncFlags returns the flag registry for the helmfile sync command.
// Sync uses the standard helmfile flags.
func SyncFlags() *flags.FlagRegistry {
	return flags.HelmfileFlags()
}

// SyncPositionalArgs builds the positional args validator for helmfile sync.
// Helmfile sync requires: sync <component>
func SyncPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
