package toolchain

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// NewLocalConfigManager creates a new local config manager.
// This is a convenience wrapper that returns the registry implementation.
func NewLocalConfigManager() *LocalConfigManager {
	defer perf.Track(nil, "toolchain.NewLocalConfigManager")()

	return registry.NewLocalConfigManager()
}

// NewLocalConfigManagerWithConfig creates a new local config manager with the given config (for testing).
func NewLocalConfigManagerWithConfig(config *LocalConfig) *LocalConfigManager {
	defer perf.Track(nil, "toolchain.NewLocalConfigManagerWithConfig")()

	return registry.NewLocalConfigManagerWithConfig(config)
}
