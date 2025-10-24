package toolchain

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
	"github.com/cloudposse/atmos/toolchain/registry/aqua"
)

// AquaRegistry is a type alias for backward compatibility.
// New code should use toolchain/registry.ToolRegistry interface.
type AquaRegistry = aqua.AquaRegistry

// NewAquaRegistry creates a new Aqua registry client.
// This is a convenience wrapper that returns the default Aqua implementation.
func NewAquaRegistry() registry.ToolRegistry {
	defer perf.Track(nil, "toolchain.NewAquaRegistry")()
	return aqua.NewAquaRegistry()
}
