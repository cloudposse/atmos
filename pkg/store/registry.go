package store

import (
	"fmt"
	"sync"
)

// StoreRegistry is a map of store name to a live store implementation.
type StoreRegistry map[string]Store

// StoreFactory builds a store backend from its configuration. Provider packages
// register a factory for each store type they implement via Register, typically
// from an init() function. The name is the configured store's key and is used
// only for diagnostics (e.g. warnings).
type StoreFactory func(name string, config StoreConfig) (Store, error)

// storeFactories holds the registered factory for each store type. It is
// populated at init time by provider packages and read at registry-build time.
// The storeFactoriesMu mutex guards it because Register is exported and may be
// called concurrently with NewStoreRegistry (e.g. late registration from tests
// or optional providers).
var (
	storeFactoriesMu sync.RWMutex
	storeFactories   = map[string]StoreFactory{}
)

// Register associates a store type identifier (e.g. "redis") with the factory
// that builds it. Provider packages call this from their init() functions, so
// importing a provider package — typically via a blank import of
// pkg/store/providers — makes its store types available to NewStoreRegistry.
//
// It panics if the same type is registered twice, which indicates a programming
// error (two factories claiming the same type).
func Register(storeType string, factory StoreFactory) {
	storeFactoriesMu.Lock()
	defer storeFactoriesMu.Unlock()

	if _, exists := storeFactories[storeType]; exists {
		panic(fmt.Sprintf("store: factory already registered for type %q", storeType))
	}

	storeFactories[storeType] = factory
}

// Reset clears all registered store factories.
//
// WARNING: This function is for TESTING ONLY. It should never be called in
// production code. It lets tests start from a clean registry state.
func Reset() {
	storeFactoriesMu.Lock()
	defer storeFactoriesMu.Unlock()

	storeFactories = map[string]StoreFactory{}
}

// NewStoreRegistry builds a registry of live stores from the provided config,
// looking up each configured store's type in the factories registered by the
// provider packages. Import the provider package (e.g. with a blank import of
// pkg/store/providers) so the built-in backends are registered before this runs.
func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for name, storeConfig := range *config {
		storeFactoriesMu.RLock()
		factory, ok := storeFactories[storeConfig.Type]
		storeFactoriesMu.RUnlock()
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

// SetAuthContextResolverWithDefaultIdentity injects an auth context resolver into
// identity-aware stores. Stores with their own configured identity keep it; stores
// without a configured identity inherit defaultIdentity.
func (r StoreRegistry) SetAuthContextResolverWithDefaultIdentity(resolver AuthContextResolver, defaultIdentity string) {
	for _, s := range r {
		ias, ok := s.(IdentityAwareStore)
		if !ok {
			continue
		}
		ias.SetAuthContext(resolver, defaultIdentityForStore(s, defaultIdentity))
	}
}

func defaultIdentityForStore(s Store, defaultIdentity string) string {
	if defaultIdentity == "" {
		return ""
	}

	identified, ok := s.(interface {
		IdentityName() string
	})
	if !ok {
		return ""
	}
	if identified.IdentityName() == "" {
		return defaultIdentity
	}

	return ""
}
