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

// StoreFactory is a function type to initialize a new store.
type StoreFactory func(options map[string]any) (Store, error)
