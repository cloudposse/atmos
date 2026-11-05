package types

import "context"

// CredentialTransaction serializes a provider session through downstream cache
// publication and logout cleanup. Providers may attach a request-scoped lease
// to the callback context to avoid reacquiring their own lock in Authenticate.
type CredentialTransaction interface {
	WithCredentialTransaction(context.Context, func(context.Context) error) error
}

// PrivateCredentialCache is implemented by providers that validate their own
// configuration-bound, durable session cache on every authentication call.
// Their public credentials must not bypass that check through the legacy
// realm/name cache. Descendants receive the freshly checked upstream result.
type PrivateCredentialCache interface {
	UsesPrivateCredentialCache() bool
}

// UsesPrivateCredentialCache reports this opt-in cache ownership contract.
func UsesPrivateCredentialCache(provider Provider) bool {
	policy, ok := provider.(PrivateCredentialCache)
	return ok && policy.UsesPrivateCredentialCache()
}

// RootProviderAware receives the resolved chain root before cache access or cleanup.
type RootProviderAware interface{ SetRootProvider(string) }
