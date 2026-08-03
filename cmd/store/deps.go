package store

import (
	pstore "github.com/cloudposse/atmos/pkg/store"
)

// storeService is the subset of *pstore.Service the command handlers depend on. It exists so
// handlers operate against an interface, letting tests inject a fake without constructing real
// config/auth. *pstore.Service satisfies it structurally -- no change to pkg/store is required.
type storeService interface {
	Set(name, stack, component, key string, value any) error
	Get(name, stack, component, key string) (any, error)
	Delete(name, stack, component, key string) error
	List() []pstore.Descriptor
}

// Seam variables wrap the real implementations so tests can override behavior. Each defaults to
// the production function; handlers call the variable, never the underlying function directly.
// Restore any override with t.Cleanup in tests.
var (
	// Loads a store service (config + auth) for the given scope.
	loadServiceFn = func(scope storeScope) (storeService, error) {
		svc, err := loadService(scope)
		if err != nil {
			return nil, err
		}
		return svc, nil
	}

	// Loads a store service for `store list`, with no authentication attempt.
	loadServiceForListFn = func(scope storeScope) (storeService, error) {
		svc, err := loadServiceForList(scope)
		if err != nil {
			return nil, err
		}
		return svc, nil
	}

	// Interactively reads a value (masked input).
	promptForValueFn = promptForStoreValue

	// Interactively confirms a destructive action.
	confirmActionFn = confirmAction
)
