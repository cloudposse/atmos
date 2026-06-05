package store

// StoreRegistry is a map of store name to store implementation. The concrete
// store backends and the NewStoreRegistry factory live in pkg/store/providers.
type StoreRegistry map[string]Store

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
