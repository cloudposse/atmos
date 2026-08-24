package keyring

import (
	"sync"
)

// memoryKeyring is an in-memory backend for tests and ephemeral use. It does not persist.
type memoryKeyring struct {
	mu    sync.RWMutex
	items map[string]string
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{items: make(map[string]string)}
}

func (s *memoryKeyring) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.items[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *memoryKeyring) Set(key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = value
	return nil
}

func (s *memoryKeyring) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, key)
	return nil
}

func (s *memoryKeyring) Has(key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.items[key]
	return ok, nil
}

func (s *memoryKeyring) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.items))
	for key := range s.items {
		keys = append(keys, key)
	}
	return keys, nil
}

func (s *memoryKeyring) Type() string {
	return TypeMemory
}
