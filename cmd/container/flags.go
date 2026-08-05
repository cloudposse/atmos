package container

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ContainerFlags returns the registry of flags shared by container subcommands.
// Runtime selection is via the global `container.runtime.provider` config and
// the ATMOS_CONTAINER_RUNTIME environment variable, both honored by the runtime
// detector, so no per-command runtime flag is needed.
func ContainerFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "container.ContainerFlags")()

	return flags.CommonFlags()
}

// WithContainerFlags returns a flags.Option that adds the container flags.
func WithContainerFlags() flags.Option {
	defer perf.Track(nil, "container.WithContainerFlags")()

	return flags.WithFlagRegistry(ContainerFlags())
}
