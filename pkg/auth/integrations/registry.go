package integrations

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]IntegrationFactory)
)

// Register adds an integration factory for a kind.
// This should be called from init() functions in integration packages.
// Panics if kind is empty or factory is nil to catch configuration errors early.
func Register(kind string, factory IntegrationFactory) {
	if kind == "" {
		panic("integration kind cannot be empty")
	}
	if factory == nil {
		panic("integration factory cannot be nil")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[kind] = factory
}

// Create instantiates an integration from config.
func Create(config *IntegrationConfig) (Integration, error) {
	defer perf.Track(nil, "integrations.Create")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	registryMu.RLock()
	factory, ok := registry[config.Config.Kind]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrUnknownIntegrationKind, config.Config.Kind)
	}

	return factory(config)
}

// ListKinds returns all registered integration kinds in sorted order.
func ListKinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// IsRegistered checks if an integration kind is registered.
func IsRegistered(kind string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[kind]
	return ok
}
