package hooks

import (
	"fmt"
	"sync"
)

// MockStore is a test implementation of the store.Store interface.
type MockStore struct {
	mu     sync.Mutex
	data   map[string]any
	setErr error
	getErr error
}

// NewMockStore creates a new mock store for testing.
func NewMockStore() *MockStore {
	return &MockStore{
		data: make(map[string]any),
	}
}

// Set stores a value in the mock store.
func (m *MockStore) Set(stack string, component string, key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.setErr != nil {
		return m.setErr
	}

	storeKey := fmt.Sprintf("%s/%s/%s", stack, component, key)
	m.data[storeKey] = value
	return nil
}

// Get retrieves a value from the mock store.
func (m *MockStore) Get(stack string, component string, key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	storeKey := fmt.Sprintf("%s/%s/%s", stack, component, key)
	value, ok := m.data[storeKey]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", storeKey)
	}
	return value, nil
}

// GetKey retrieves a value by key from the mock store.
func (m *MockStore) GetKey(key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	value, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

// SetSetError configures the mock to return an error on Set calls.
func (m *MockStore) SetSetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setErr = err
}

// SetGetError configures the mock to return an error on Get calls.
func (m *MockStore) SetGetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getErr = err
}

// GetData returns all stored data for verification.
func (m *MockStore) GetData() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	dataCopy := make(map[string]any)
	for k, v := range m.data {
		dataCopy[k] = v
	}
	return dataCopy
}

// Clear removes all data from the mock store.
func (m *MockStore) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]any)
	m.setErr = nil
	m.getErr = nil
}
