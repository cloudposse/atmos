package emulator

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EmulatorFlags returns the registry of flags shared by emulator subcommands.
// Runtime selection is via the global `container.runtime.provider` config and the
// ATMOS_CONTAINER_RUNTIME environment variable, both honored by the runtime
// detector, so no per-command runtime flag is needed.
func EmulatorFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "emulator.EmulatorFlags")()

	return flags.CommonFlags()
}

// WithEmulatorFlags returns a flags.Option that adds the emulator flags.
func WithEmulatorFlags() flags.Option {
	defer perf.Track(nil, "emulator.WithEmulatorFlags")()

	return flags.WithFlagRegistry(EmulatorFlags())
}
