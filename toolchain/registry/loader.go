package registry

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registryFactory is a function type that creates registry instances.
// This allows the aqua package to register itself without creating circular dependencies.
type registryFactory func() ToolRegistry

// atmosRegistryFactory is a function type that creates inline registries from config.
// This allows the atmos package to register itself without creating circular dependencies.
type atmosRegistryFactory func(tools map[string]any) (ToolRegistry, error)

var (
	// DefaultRegistryFactory is the factory for the default registry (Aqua).
	// Set by the aqua package's init() function.
	defaultRegistryFactory registryFactory

	// AtmosRegistryConstructor is the factory for creating inline registries.
	// Set by the atmos package's init() function.
	atmosRegistryConstructor atmosRegistryFactory
)

// RegisterDefaultRegistry allows a registry implementation to register itself as the default.
// This is called by aqua package during initialization.
func RegisterDefaultRegistry(factory registryFactory) {
	defer perf.Track(nil, "registry.RegisterDefaultRegistry")()

	defaultRegistryFactory = factory
}

// RegisterAtmosRegistry allows the atmos package to register its constructor.
// This is called by atmos package during initialization.
func RegisterAtmosRegistry(factory atmosRegistryFactory) {
	defer perf.Track(nil, "registry.RegisterAtmosRegistry")()

	atmosRegistryConstructor = factory
}

// LoadFromConfig creates a ToolRegistry from an Atmos configuration.
// Returns a CompositeRegistry that coordinates multiple registry sources.
// If no registries are configured, returns a default Aqua registry.
func LoadFromConfig(atmosConfig *schema.AtmosConfiguration) (ToolRegistry, error) {
	defer perf.Track(atmosConfig, "registry.LoadFromConfig")()

	// Backward compatibility: If no registries are configured, use default registry.
	if len(atmosConfig.Toolchain.Registries) == 0 {
		if defaultRegistryFactory == nil {
			return nil, fmt.Errorf("%w: no default registry factory registered", ErrRegistryNotRegistered)
		}
		return defaultRegistryFactory(), nil
	}

	// Build list of prioritized registries.
	var registries []PrioritizedRegistry

	for _, regConfig := range atmosConfig.Toolchain.Registries {
		reg, err := createRegistry(regConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create registry %q: %w", regConfig.Name, err)
		}

		registries = append(registries, PrioritizedRegistry{
			Name:     regConfig.Name,
			Registry: reg,
			Priority: regConfig.Priority,
		})
	}

	return NewCompositeRegistry(registries), nil
}

// createRegistry creates a registry instance based on the configuration.
func createRegistry(config schema.ToolchainRegistry) (ToolRegistry, error) {
	defer perf.Track(nil, "registry.createRegistry")()

	switch config.Type {
	case "aqua":
		// Aqua registry format/schema.
		// Source is optional - defaults to official Aqua registry if not specified.
		if config.Source == "" {
			// Official Aqua registry (default).
			if defaultRegistryFactory == nil {
				return nil, fmt.Errorf("%w: no default registry factory registered", ErrRegistryNotRegistered)
			}
			return defaultRegistryFactory(), nil
		}
		// Custom Aqua-format registry at specified URL (e.g., corporate registry, mirror).
		return NewURLRegistry(config.Source), nil

	case "atmos":
		// Inline Atmos-format registry (tools defined directly in config).
		if config.Tools == nil {
			return nil, fmt.Errorf("%w: registry type 'atmos' requires 'tools' field", ErrRegistryConfiguration)
		}
		// Create AtmosRegistry from inline tools config using registered factory.
		if atmosRegistryConstructor == nil {
			return nil, fmt.Errorf("%w: atmos registry constructor not registered", ErrRegistryNotRegistered)
		}
		return atmosRegistryConstructor(config.Tools)

	default:
		return nil, fmt.Errorf("%w: %s (supported types: 'aqua', 'atmos')", ErrUnknownRegistry, config.Type)
	}
}
