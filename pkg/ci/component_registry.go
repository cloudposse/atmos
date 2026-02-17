package ci

import (
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	componentProvidersMu sync.RWMutex
	componentProviders   = make(map[string]ComponentCIProvider)
)

// RegisterComponentProvider registers a component CI provider.
// Providers should call this in their init() function for self-registration.
func RegisterComponentProvider(p ComponentCIProvider) error {
	defer perf.Track(nil, "ci.RegisterComponentProvider")()

	if p == nil {
		return errUtils.ErrNilParam
	}

	componentType := p.GetType()
	if componentType == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("ComponentCIProvider has empty type").
			Err()
	}

	componentProvidersMu.Lock()
	defer componentProvidersMu.Unlock()

	if _, exists := componentProviders[componentType]; exists {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("ComponentCIProvider already registered").
			WithContext("component_type", componentType).
			Err()
	}

	componentProviders[componentType] = p
	return nil
}

// GetComponentProvider returns a component CI provider by type.
func GetComponentProvider(componentType string) (ComponentCIProvider, bool) {
	defer perf.Track(nil, "ci.GetComponentProvider")()

	componentProvidersMu.RLock()
	defer componentProvidersMu.RUnlock()

	p, ok := componentProviders[componentType]
	return p, ok
}

// GetComponentProviderForEvent returns the provider that handles a specific hook event.
// Returns nil if no provider handles the event.
func GetComponentProviderForEvent(event string) ComponentCIProvider {
	defer perf.Track(nil, "ci.GetComponentProviderForEvent")()

	componentProvidersMu.RLock()
	defer componentProvidersMu.RUnlock()

	for _, p := range componentProviders {
		bindings := HookBindings(p.GetHookBindings())
		if bindings.GetBindingForEvent(event) != nil {
			return p
		}
	}
	return nil
}

// ListComponentProviders returns all registered component provider types.
func ListComponentProviders() []string {
	defer perf.Track(nil, "ci.ListComponentProviders")()

	componentProvidersMu.RLock()
	defer componentProvidersMu.RUnlock()

	types := make([]string, 0, len(componentProviders))
	for t := range componentProviders {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// ClearComponentProviders removes all registered component providers.
// This is primarily for testing.
func ClearComponentProviders() {
	defer perf.Track(nil, "ci.ClearComponentProviders")()

	componentProvidersMu.Lock()
	defer componentProvidersMu.Unlock()
	componentProviders = make(map[string]ComponentCIProvider)
}
