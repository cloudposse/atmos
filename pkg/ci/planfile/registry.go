package planfile

import (
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	registryMu sync.RWMutex
	factories  = make(map[string]StoreFactory)
)

// Register registers a store factory for the given type.
// Both storeType and factory must be non-empty/non-nil.
func Register(storeType string, factory StoreFactory) {
	defer perf.Track(nil, "planfile.Register")()

	if storeType == "" {
		panic(fmt.Sprintf("%v: store type cannot be empty", errUtils.ErrPlanfileStoreInvalidArgs))
	}
	if factory == nil {
		panic(fmt.Sprintf("%v: factory cannot be nil for store type %q", errUtils.ErrPlanfileStoreInvalidArgs, storeType))
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	factories[storeType] = factory
}

// NewStore creates a new store from the given options.
func NewStore(opts StoreOptions) (Store, error) {
	defer perf.Track(opts.AtmosConfig, "planfile.NewStore")()

	registryMu.RLock()
	factory, ok := factories[opts.Type]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileStoreNotFound, opts.Type)
	}

	return factory(opts)
}

// GetRegisteredTypes returns a list of registered store types.
func GetRegisteredTypes() []string {
	defer perf.Track(nil, "planfile.GetRegisteredTypes")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for t := range factories {
		types = append(types, t)
	}
	return types
}
