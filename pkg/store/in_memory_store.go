package store

import (
	"fmt"
)

// InMemoryStore implements the Store interface with an in-memory map
type InMemoryStore struct {
	data map[string]interface{}
}

// NewInMemoryStore initializes a new MemoryStore
func NewInMemoryStore() (Store, error) {
	return &InMemoryStore{
		data: make(map[string]interface{}),
	}, nil
}

// getKey generates a consistent key format for the in-memory store
func (s *InMemoryStore) getKey(stack, component, key string) string {
	return fmt.Sprintf("%s/%s/%s", stack, component, key)
}

// Set stores a value in memory
func (s *InMemoryStore) Set(stack, component, key string, value interface{}) error {
	if stack == "" {
		return fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	fullKey := s.getKey(stack, component, key)
	s.data[fullKey] = value
	return nil
}

// Get retrieves a value from memory
func (s *InMemoryStore) Get(stack, component, key string) (interface{}, error) {
	if stack == "" {
		return nil, fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return nil, fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	fullKey := s.getKey(stack, component, key)
	value, exists := s.data[fullKey]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", fullKey)
	}
	return value, nil
}
