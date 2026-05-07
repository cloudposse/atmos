package artifact

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

// Register registers a backend factory for the given type.
// Both storeType and factory must be non-empty/non-nil.
func Register(storeType string, factory BackendFactory) {
	defer perf.Track(nil, "artifact.Register")()

	if storeType == "" {
		panic(fmt.Sprintf("%v: store type cannot be empty", errUtils.ErrArtifactStoreInvalidArgs))
	}
	if factory == nil {
		panic(fmt.Sprintf("%v: factory cannot be nil for store type %q", errUtils.ErrArtifactStoreInvalidArgs, storeType))
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	factories[storeType] = factory
}

// NewStore creates a new store from the given options.
// It creates a Backend via the registered factory and wraps it in a BundledStore.
func NewStore(opts StoreOptions) (Store, error) {
	defer perf.Track(opts.AtmosConfig, "artifact.NewStore")()

	registryMu.RLock()
	factory, ok := factories[opts.Type]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactStoreNotFound, opts.Type)
	}

	backend, err := factory(opts)
	if err != nil {
		return nil, err
	}

	return NewBundledStore(backend), nil
}

// GetRegisteredTypes returns a list of registered store types.
func GetRegisteredTypes() []string {
	defer perf.Track(nil, "artifact.GetRegisteredTypes")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for t := range factories {
		types = append(types, t)
	}
	return types
}
