package registry

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registryFactory is a function type that creates registry instances.
// This allows the aqua package to register itself without creating circular dependencies.
type registryFactory func() ToolRegistry

// atmosRegistryFactory is a function type that creates inline registries from config.
// This allows the atmos package to register itself without creating circular dependencies.
type atmosRegistryFactory func(tools map[string]any) (ToolRegistry, error)

// builtinAtmosRegistryFactory is a function type that returns the Atmos
// curated registry baked into the binary. Used to break the cyclic import
// between this package and pkg/toolchain/registry/atmos (which depends on
// types defined here).
type builtinAtmosRegistryFactory func() (ToolRegistry, error)

var (
	// DefaultRegistryFactory is the factory for the default registry (Aqua).
	// Set by the aqua package's init() function.
	defaultRegistryFactory registryFactory

	// AtmosRegistryConstructor is the factory for creating inline registries.
	// Set by the atmos package's init() function.
	atmosRegistryConstructor atmosRegistryFactory

	// The builtinAtmosFactory returns the Atmos curated registry of tool
	// overrides shipped with the binary (e.g., KICS). Set by the atmos
	// package's init() function via RegisterBuiltinAtmosRegistry.
	builtinAtmosFactory builtinAtmosRegistryFactory
)

// RegisterDefaultRegistry allows a registry implementation to register itself as the default.
// This is called by aqua package during initialization.
func RegisterDefaultRegistry(factory registryFactory) {
	defer perf.Track(nil, "registry.RegisterDefaultRegistry")()

	defaultRegistryFactory = factory
}

// DefaultRegistry returns a fresh instance of the registered default registry
// (Aqua), or nil if none has been registered. Use this to access registry
// capabilities (e.g., short-name resolution) from callers that don't have a
// full atmos configuration available.
func DefaultRegistry() ToolRegistry {
	defer perf.Track(nil, "registry.DefaultRegistry")()

	if defaultRegistryFactory == nil {
		return nil
	}
	return defaultRegistryFactory()
}

// RegisterAtmosRegistry allows the atmos package to register its constructor.
// This is called by atmos package during initialization.
func RegisterAtmosRegistry(factory atmosRegistryFactory) {
	defer perf.Track(nil, "registry.RegisterAtmosRegistry")()

	atmosRegistryConstructor = factory
}

// RegisterBuiltinAtmosRegistry allows the atmos package to register a
// factory that returns the curated registry baked into the binary.
// Called by atmos package init().
func RegisterBuiltinAtmosRegistry(factory builtinAtmosRegistryFactory) {
	defer perf.Track(nil, "registry.RegisterBuiltinAtmosRegistry")()

	builtinAtmosFactory = factory
}

// BuiltinAtmosRegistryPriority is the priority assigned to the curated
// Atmos registry in the composite. Above Aqua's default 10 so overrides
// win, low enough that users can still override even the built-ins with
// a higher-priority registry in atmos.yaml.
const BuiltinAtmosRegistryPriority = 100

// LoadFromConfig creates a ToolRegistry from an Atmos configuration.
// Returns a CompositeRegistry that coordinates multiple registry sources.
//
// The composite ALWAYS includes the Atmos curated built-in registry (when
// registered by pkg/toolchain/registry/atmos) at high priority — that's
// where overrides for tools the upstream Aqua registry doesn't model well
// live (e.g., KICS). User-configured registries from atmos.yaml stack on
// top; users can override a built-in entry by configuring a registry with
// priority > BuiltinAtmosRegistryPriority.
//
// If atmos.yaml has no `toolchain.registries` block, the result is the
// built-in registry composited with the default Aqua registry. This
// preserves the zero-config behavior while making the curated overrides
// available without YAML changes.
func LoadFromConfig(atmosConfig *schema.AtmosConfiguration) (ToolRegistry, error) {
	defer perf.Track(atmosConfig, "registry.LoadFromConfig")()

	var registries []PrioritizedRegistry

	// Always include the built-in Atmos curated registry when its factory
	// has been registered. Skipping it would silently drop overrides for
	// tools that depend on those entries (KICS today; potentially more).
	if builtinAtmosFactory != nil {
		builtinReg, err := builtinAtmosFactory()
		if err != nil {
			return nil, fmt.Errorf("failed to load built-in atmos registry: %w", err)
		}
		registries = append(registries, PrioritizedRegistry{
			Name:     "atmos-builtin",
			Registry: builtinReg,
			Priority: BuiltinAtmosRegistryPriority,
		})
	}

	// No user-configured registries: fall back to the default Aqua registry
	// alongside the built-in.
	if len(atmosConfig.Toolchain.Registries) == 0 {
		if defaultRegistryFactory == nil {
			return nil, fmt.Errorf("%w: no default registry factory registered", ErrRegistryNotRegistered)
		}
		registries = append(registries, PrioritizedRegistry{
			Name:     "aqua-public",
			Registry: defaultRegistryFactory(),
			Priority: 10,
		})
		return NewCompositeRegistry(registries), nil
	}

	// User configured one or more registries: append them after the built-in.
	for i := range atmosConfig.Toolchain.Registries {
		regConfig := &atmosConfig.Toolchain.Registries[i]
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
func createRegistry(config *schema.ToolchainRegistry) (ToolRegistry, error) {
	defer perf.Track(nil, "registry.createRegistry")()

	switch config.Type {
	case "aqua":
		return createAquaRegistry(config)
	case "atmos":
		return createAtmosRegistry(config)
	default:
		return nil, fmt.Errorf("%w: %s (supported types: 'aqua', 'atmos')", ErrUnknownRegistry, config.Type)
	}
}

// createAquaRegistry creates an Aqua-format registry from the configuration.
func createAquaRegistry(config *schema.ToolchainRegistry) (ToolRegistry, error) {
	// Source is optional - defaults to official Aqua registry if not specified.
	if config.Source == "" {
		return createDefaultAquaRegistry(config.Ref)
	}
	// Validate ref is only used with GitHub URLs.
	if config.Ref != "" && !isGitHubURL(config.Source) {
		return nil, fmt.Errorf("%w: 'ref' is only supported for github.com URLs, got %q", ErrRegistryConfiguration, config.Source)
	}
	// Custom Aqua-format registry at specified URL (e.g., corporate registry, mirror).
	// If ref is provided, it will be used to pin the registry to a specific Git ref.
	return NewURLRegistry(config.Source, config.Ref), nil
}

// createDefaultAquaRegistry creates the official Aqua registry (default).
func createDefaultAquaRegistry(ref string) (ToolRegistry, error) {
	if ref != "" {
		return nil, fmt.Errorf("%w: 'ref' requires 'source' to be set", ErrRegistryConfiguration)
	}
	if defaultRegistryFactory == nil {
		return nil, fmt.Errorf("%w: no default registry factory registered", ErrRegistryNotRegistered)
	}
	return defaultRegistryFactory(), nil
}

// createAtmosRegistry creates an inline Atmos-format registry from the configuration.
func createAtmosRegistry(config *schema.ToolchainRegistry) (ToolRegistry, error) {
	if config.Tools == nil {
		return nil, fmt.Errorf("%w: registry type 'atmos' requires 'tools' field", ErrRegistryConfiguration)
	}
	if atmosRegistryConstructor == nil {
		return nil, fmt.Errorf("%w: atmos registry constructor not registered", ErrRegistryNotRegistered)
	}
	return atmosRegistryConstructor(config.Tools)
}

// isGitHubURL checks if the URL is a github.com URL (not raw.githubusercontent.com).
func isGitHubURL(url string) bool {
	return strings.Contains(url, "github.com") && !strings.Contains(url, "raw.githubusercontent.com")
}
