package store

// Store defines the common interface for all store implementations.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_store.go -package=store
type Store interface {
	// Set stores a value for a specific stack, component, and key combination.
	Set(stack string, component string, key string, value any) error
	// Get retrieves a value for a specific stack, component, and key combination.
	Get(stack string, component string, key string) (any, error)
	// GetKey retrieves a value directly by key without stack or component context.
	GetKey(key string) (any, error)
}

// DeletableStore extends Store with the ability to remove a value. Backends that support
// deletion (SSM, ASM, Vault, Azure Key Vault, GCP Secret Manager) implement this; backends
// that don't may return ErrDeleteNotSupported. The secrets CLI (`atmos secret delete`)
// requires it.
type DeletableStore interface {
	Store
	// Delete removes the value for a specific stack, component, and key combination.
	Delete(stack string, component string, key string) error
}

// StatusStore extends Store with an existence check used by `atmos secret list`/`validate`
// to report whether a declared secret has been initialized.
//
// Has MUST determine existence without retrieving or decrypting the value: it uses a
// metadata/describe API (e.g. SSM GetParameter with WithDecryption=false, Secrets Manager
// DescribeSecret, GCP GetSecretVersion) so that listing never requires a decrypt-capable
// identity (no kms:Decrypt) and never registers a plaintext value with the masker.
type StatusStore interface {
	Store
	// Has reports whether a value exists for a specific stack, component, and key, without
	// retrieving or decrypting the value.
	Has(stack string, component string, key string) (bool, error)
}

// LocalStore is an optional marker for stores whose existence check (Has) needs no network
// access and no authentication — e.g. the OS keychain. `atmos secret list` treats local
// stores as always-safe to check (free), and reports non-local (remote) stores as Unknown
// unless verification is explicitly requested (`--verify`). Remote stores must NOT implement it.
type LocalStore interface {
	Store
	// IsLocal reports whether the store operates without network access or authentication.
	IsLocal() bool
}

// SecretAwareStore is implemented by stores that change their at-rest behavior when used as
// a secret backend (e.g. AWS SSM writes a SecureString instead of a String). The registry
// calls SetSecret(true) for stores configured with `secret: true`.
type SecretAwareStore interface {
	Store
	// SetSecret marks the store as a secret backend so writes use the sensitive at-rest variant.
	SetSecret(secret bool)
}

// ListableStore is implemented by stores that can enumerate the keys stored under a
// stack/component scope (or globally when both are empty). Not every backend can enumerate keys
// cheaply or safely (see each provider's Keys implementation for details) — 1Password never
// implements this (its addressing is opaque op:// reference templates, not the getKey() scheme
// every other backend shares), and the keychain store's default (system/OS) backend returns
// ErrListNotSupported at runtime even though it implements the interface. Keys returns key names
// only; fetch a value with Get/GetKey.
type ListableStore interface {
	Store
	// Keys lists the keys under a stack/component scope (or globally when both are empty).
	Keys(stack, component string) ([]string, error)
}

// ValueListableStore is implemented by stores whose ListableStore.Keys enumeration is only safe
// to pair with per-key Get calls in some execution contexts. GitHubActionsStore's Get requires a
// GitHub Actions runner; outside one it implements this to report false so ListKeyValues can
// fail fast with ErrListNotSupported instead of aborting mid-enumeration on the first Get error.
type ValueListableStore interface {
	Store
	// ValueListingSupported reports whether Get can currently be called for every key Keys
	// returns.
	ValueListingSupported() bool
}
