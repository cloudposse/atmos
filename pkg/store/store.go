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
// to report whether a declared secret has been initialized, without retrieving (and thus
// without registering) its value.
type StatusStore interface {
	Store
	// Has reports whether a value exists for a specific stack, component, and key.
	Has(stack string, component string, key string) (bool, error)
}

// SecretAwareStore is implemented by stores that change their at-rest behavior when used as
// a secret backend (e.g. AWS SSM writes a SecureString instead of a String). The registry
// calls SetSecret(true) for stores configured with `secret: true`.
type SecretAwareStore interface {
	Store
	// SetSecret marks the store as a secret backend so writes use the sensitive at-rest variant.
	SetSecret(secret bool)
}
