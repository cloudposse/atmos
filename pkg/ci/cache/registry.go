package cache

import (
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	registryMu sync.RWMutex
	factories  = make(map[string]BackendFactory)
)

// Register registers a cache backend factory for the given type.
// Backends call this in their init() function. Both backendType and factory
// must be non-empty/non-nil.
func Register(backendType string, factory BackendFactory) {
	defer perf.Track(nil, "cache.Register")()

	if backendType == "" {
		panic(fmt.Sprintf("%v: backend type cannot be empty", errUtils.ErrCacheInvalidArgs))
	}
	if factory == nil {
		panic(fmt.Sprintf("%v: factory cannot be nil for backend type %q", errUtils.ErrCacheInvalidArgs, backendType))
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	factories[backendType] = factory
}

// NewBackend creates a backend of the given type from the registered factory.
func NewBackend(backendType string, opts Options) (Backend, error) {
	defer perf.Track(opts.AtmosConfig, "cache.NewBackend")()

	registryMu.RLock()
	factory, ok := factories[backendType]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrCacheBackendNotFound, backendType)
	}

	return factory(opts)
}

// GetRegisteredTypes returns the list of registered cache backend types.
func GetRegisteredTypes() []string {
	defer perf.Track(nil, "cache.GetRegisteredTypes")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for t := range factories {
		types = append(types, t)
	}
	return types
}
