package deferred

import (
	"fmt"
	"reflect"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

type storeOperation struct {
	mu    sync.Mutex
	users int
}

// Key by backend instance, not registry name/configuration: aliases and copied
// configurations can refer to the same mutable client. Entries live only while
// there are holders or waiters, so discarded invocations do not retain stores.
var storeOperations = struct {
	sync.Mutex
	entries map[store.IdentityAwareStore]*storeOperation
}{entries: make(map[store.IdentityAwareStore]*storeOperation)}

// Non-comparable third-party implementations cannot be map keys; conservatively
// serialize those with each other. Built-in providers are comparable pointers.
var nonComparableStoreOperation sync.Mutex

// WithStoreAuth keeps credential binding and the operation on that binding atomic.
// The callback must not recursively access the same store; parsing and value/query
// processing belong outside this boundary. Unused lookups do not acquire it.
func WithStoreAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, name string, operation func() (any, error)) (any, error) {
	defer perf.Track(ac, "store.deferred.WithStoreAuth")()
	if backend, ok := ac.Stores[name].(store.IdentityAwareStore); ok {
		unlock := lockStore(backend)
		defer unlock()
	}
	if err := resolveStoreAuth(ac, info, name); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, err)
	}
	return operation()
}

func lockStore(backend store.IdentityAwareStore) func() {
	if !reflect.ValueOf(backend).Comparable() {
		nonComparableStoreOperation.Lock()
		return nonComparableStoreOperation.Unlock
	}
	storeOperations.Lock()
	entry := storeOperations.entries[backend]
	if entry == nil {
		entry = &storeOperation{}
		storeOperations.entries[backend] = entry
	}
	entry.users++
	storeOperations.Unlock()
	entry.mu.Lock()
	return func() {
		storeOperations.Lock()
		entry.mu.Unlock()
		entry.users--
		if entry.users == 0 {
			delete(storeOperations.entries, backend)
		}
		storeOperations.Unlock()
	}
}
