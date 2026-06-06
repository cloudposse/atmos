package store

import "fmt"

// StoreRegistry is a map of store name to a live store implementation.
type StoreRegistry map[string]Store

// StoreFactory builds a store backend from its configuration. Provider packages
// register a factory for each store type they implement via Register, typically
// from an init() function. The name is the configured store's key and is used
// only for diagnostics (e.g. warnings).
type StoreFactory func(name string, config StoreConfig) (Store, error)

// storeFactories holds the registered factory for each store type. It is
// populated at init time by provider packages and read at registry-build time.
var storeFactories = map[string]StoreFactory{}

// Register associates a store type identifier (e.g. "redis") with the factory
// that builds it. Provider packages call this from their init() functions, so
// importing a provider package — typically via a blank import of
// pkg/store/providers — makes its store types available to NewStoreRegistry.
//
// It panics if the same type is registered twice, which indicates a programming
// error (two factories claiming the same type).
func Register(storeType string, factory StoreFactory) {
	if _, exists := storeFactories[storeType]; exists {
		panic(fmt.Sprintf("store: factory already registered for type %q", storeType))
	}

	storeFactories[storeType] = factory
}

// NewStoreRegistry builds a registry of live stores from the provided config,
// looking up each configured store's type in the factories registered by the
// provider packages. Import the provider package (e.g. with a blank import of
// pkg/store/providers) so the built-in backends are registered before this runs.
func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for name, storeConfig := range *config {
		factory, ok := storeFactories[storeConfig.Type]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrStoreTypeNotFound, storeConfig.Type)
		}

		s, err := factory(name, storeConfig)
		if err != nil {
			return nil, err
		}
		registry[name] = s
	}

	return registry, nil
}

// SetAuthContextResolver injects an auth context resolver into all identity-aware stores
// that have an identity configured. This should be called after authentication is complete
// and before stores are accessed.
func (r StoreRegistry) SetAuthContextResolver(resolver AuthContextResolver) {
	for _, s := range r {
		if ias, ok := s.(IdentityAwareStore); ok {
			// Pass empty identity name — the store already has its identity name from construction.
			// SetAuthContext will only update the resolver, not override a non-empty identity.
			ias.SetAuthContext(resolver, "")
		}
	}
}
