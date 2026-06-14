package store

import "strings"

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

// StoreFactory is a function type to initialize a new store.
type StoreFactory func(options map[string]any) (Store, error)

// nolint
// getKey generates a key for the store. First it splits the stack by the stack delimiter (from atmos.yaml),
// then it splits the component if it contains a "/",
// then it appends the key to the parts,
// then it joins the parts with the final delimiter.
//
// Empty segments are omitted entirely — independent of the final delimiter — so scoped secret
// coordinates collapse cleanly: an empty component (a stack-scoped secret) yields
// `prefix<delim>stack<delim>key`, and an empty stack and component (a global secret) yields
// `prefix<delim>key`.
func getKey(prefix string, stackDelimiter string, stack string, component string, key string, finalDelimiter string) (string, error) { //nolint
	parts := []string{prefix}
	if stack != "" {
		parts = append(parts, strings.Split(stack, stackDelimiter)...)
	}
	if component != "" {
		parts = append(parts, strings.Split(component, "/")...)
	}
	parts = append(parts, key)

	joinedKey := strings.Join(parts, finalDelimiter)
	finalKey := strings.ReplaceAll(joinedKey, "//", "/")

	return finalKey, nil
}
