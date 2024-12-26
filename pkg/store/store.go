package store

// Store defines the common interface for all store implementations.
type Store interface {
	Set(key string, value interface{}) error
	Get(key string) (interface{}, error) // Default values if it doesn't exist?
}

// StoreFactory is a function type to initialize a new store.
type StoreFactory func(options map[string]interface{}) (Store, error)
