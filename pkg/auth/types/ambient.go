package types

// AmbientProvider is the optional interface implemented by providers that resolve
// credentials live from ambient environment state on every authentication rather than
// owning durable credentials of their own.
//
// Examples are `gcp/adc` (resolves Application Default Credentials via
// GOOGLE_APPLICATION_CREDENTIALS, the gcloud config, or the metadata server) and
// `gcp/workload-identity-federation` (exchanges an OIDC token pulled from the
// environment, a file, or a URL). Both mint short-lived access tokens that are a
// *snapshot* of whatever principal the ambient environment resolved to at that moment.
//
// The auth manager treats such chains as non-cacheable: it never persists their
// credentials to the keyring and never reuses persisted entries. Persisting them
// breaks the provider's contract — after the ambient identity changes (e.g. a fresh
// `gcloud auth application-default login` as a different account) the manager would
// keep replaying the stale token instead of re-resolving. The process-level in-memory
// cache still applies, so a single Atmos command performs at most one resolution.
//
// Providers that do NOT implement this interface are cached as before; the manager
// checks the interface dynamically, so adding a new ambient provider requires no edit
// to the generic manager.
type AmbientProvider interface {
	// IsAmbient reports whether this provider resolves credentials from ambient
	// environment state on every authentication. Implementations that return false
	// are cached normally.
	IsAmbient() bool
}

// ProviderIsAmbient reports whether the given provider resolves credentials from
// ambient environment state on every authentication. Returns false for a nil provider
// or one that does not implement AmbientProvider.
func ProviderIsAmbient(provider Provider) bool {
	ambient, ok := provider.(AmbientProvider)
	return ok && ambient.IsAmbient()
}
