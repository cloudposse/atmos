package store

import (
	"fmt"
	"sync"
)

// InMemoryStore is an in-memory store implementation.
type InMemoryStore struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

// Ensure MemoryStore implements the Store interface.
var _ Store = (*InMemoryStore)(nil)

// NewInMemoryStore initializes a new MemoryStore.
func NewInMemoryStore(options map[string]interface{}) (Store, error) {
	return &InMemoryStore{data: make(map[string]interface{})}, nil
}

// Set stores a key-value pair in memory.
func (m *InMemoryStore) Set(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

// Get retrieves a value by key from memory.
func (m *InMemoryStore) Get(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.data[key]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", key)
	}
	return value, nil
}
