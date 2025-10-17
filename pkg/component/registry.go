package component

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

var (
	// Global registry instance.
	registry = &ComponentRegistry{
		providers: make(map[string]ComponentProvider),
	}
)

// ComponentRegistry manages component provider registration.
// It is thread-safe and supports concurrent registration and access.
type ComponentRegistry struct {
	mu        sync.RWMutex
	providers map[string]ComponentProvider
}

// Register adds a component provider to the registry.
// This is called during package init() for built-in components.
// If a provider with the same type already exists, it will be replaced
// (last registration wins, allowing plugin override).
//
// Returns an error if the provider is nil or has an empty type.
func Register(provider ComponentProvider) error {
	if provider == nil {
		return errUtils.ErrComponentProviderNil
	}

	componentType := provider.GetType()
	if componentType == "" {
		return errUtils.ErrComponentTypeEmpty
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers[componentType] = provider
	return nil
}

// GetProvider returns a component provider by type.
// Returns the provider and true if found, nil and false otherwise.
func GetProvider(componentType string) (ComponentProvider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	provider, ok := registry.providers[componentType]
	return provider, ok
}

// ListProviders returns all registered providers grouped by category.
// The map key is the group name, and the value is a slice of providers in that group.
func ListProviders() map[string][]ComponentProvider {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	grouped := make(map[string][]ComponentProvider)

	for _, provider := range registry.providers {
		group := provider.GetGroup()
		grouped[group] = append(grouped[group], provider)
	}

	return grouped
}

// ListTypes returns all registered component types sorted alphabetically.
// Example: ["helmfile", "mock", "packer", "terraform"]
func ListTypes() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	types := make([]string, 0, len(registry.providers))
	for componentType := range registry.providers {
		types = append(types, componentType)
	}

	sort.Strings(types)
	return types
}

// Count returns the number of registered providers.
func Count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return len(registry.providers)
}

// GetInfo returns metadata for all registered component providers.
// Results are sorted by component type for consistent ordering.
func GetInfo() []ComponentInfo {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	infos := make([]ComponentInfo, 0, len(registry.providers))
	for _, provider := range registry.providers {
		infos = append(infos, ComponentInfo{
			Type:     provider.GetType(),
			Group:    provider.GetGroup(),
			Commands: provider.GetAvailableCommands(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Type < infos[j].Type
	})

	return infos
}

// Reset clears the registry (for testing only).
// This should not be used in production code.
func Reset() {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers = make(map[string]ComponentProvider)
}

// MustGetProvider returns a component provider by type or panics if not found.
// This is useful in contexts where the component type is known to be registered.
func MustGetProvider(componentType string) ComponentProvider {
	provider, ok := GetProvider(componentType)
	if !ok {
		panic(fmt.Errorf("%w: %s", errUtils.ErrComponentProviderNotFound, componentType))
	}
	return provider
}
