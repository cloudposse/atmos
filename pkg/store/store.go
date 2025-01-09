package store

// Store defines the common interface for all store implementations.
type Store interface {
	Set(stack string, component string, key string, value interface{}) error
	Get(stack string, component string, key string) (interface{}, error)
}

// StoreFactory is a function type to initialize a new store.
type StoreFactory func(options map[string]interface{}) (Store, error)
