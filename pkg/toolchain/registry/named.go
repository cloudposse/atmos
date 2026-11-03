package registry

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfiguredRegistryInfo describes a registry without fetching its tool catalog.
type ConfiguredRegistryInfo struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Priority int    `json:"priority" yaml:"priority"`
}

// ConfiguredRegistries lists the registries used by LoadFromConfig.
func ConfiguredRegistries(config *schema.AtmosConfiguration) []ConfiguredRegistryInfo {
	defer perf.Track(config, "registry.ConfiguredRegistries")()
	result := []ConfiguredRegistryInfo{}
	if builtinAtmosFactory != nil {
		result = append(result, ConfiguredRegistryInfo{Name: "atmos-builtin", Type: "atmos", Priority: BuiltinAtmosRegistryPriority})
	}
	if config == nil || len(config.Toolchain.Registries) == 0 {
		return append(result, ConfiguredRegistryInfo{Name: "aqua-public", Type: "aqua", Priority: 10})
	}
	for _, entry := range config.Toolchain.Registries {
		result = append(result, ConfiguredRegistryInfo{Name: entry.Name, Type: entry.Type, Priority: entry.Priority})
	}
	return result
}

// LoadNamedFromConfig loads a single configured registry, preserving the aqua
// shorthand and built-in registry names used by registry inspection commands.
func LoadNamedFromConfig(config *schema.AtmosConfiguration, name string) (ToolRegistry, error) {
	defer perf.Track(config, "registry.LoadNamedFromConfig")()
	if entry := findConfiguredRegistry(config, name); entry != nil {
		return createRegistry(entry)
	}

	switch name {
	case "aqua", "aqua-public":
		if reg := DefaultRegistry(); reg != nil {
			return reg, nil
		}
		return nil, fmt.Errorf("%w: no default Aqua registry registered", ErrRegistryNotRegistered)
	case "atmos-builtin":
		if builtinAtmosFactory != nil {
			return builtinAtmosFactory()
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownRegistry, name)
}

func findConfiguredRegistry(config *schema.AtmosConfiguration, name string) *schema.ToolchainRegistry {
	if config == nil {
		return nil
	}
	for i := range config.Toolchain.Registries {
		entry := &config.Toolchain.Registries[i]
		if entry.Name == name {
			return entry
		}
	}
	if name == "aqua" {
		return findConfiguredRegistry(config, "aqua-public")
	}
	return nil
}
