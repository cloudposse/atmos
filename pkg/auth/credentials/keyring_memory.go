package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// memoryKeyringStore implements the CredentialStore interface using in-memory storage.
// This backend is intended for testing only and does not persist data.
type memoryKeyringStore struct {
	mu    sync.RWMutex
	items map[string]string // alias -> JSON data
}

// newMemoryKeyringStore creates a new in-memory keyring store.
func newMemoryKeyringStore() (*memoryKeyringStore, error) {
	return &memoryKeyringStore{
		items: make(map[string]string),
	}, nil
}

// Store stores credentials for the given alias.
func (s *memoryKeyringStore) Store(alias string, creds types.ICredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		typ string
		raw []byte
		err error
	)

	switch c := creds.(type) {
	case *types.AWSCredentials:
		typ = "aws"
		raw, err = json.Marshal(c)
	case *types.OIDCCredentials:
		typ = "oidc"
		raw, err = json.Marshal(c)
	default:
		return errors.Join(ErrCredentialStore, fmt.Errorf("unsupported credential type %T", creds))
	}
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	env := credentialEnvelope{Type: typ, Data: raw}
	data, err := json.Marshal(&env)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	s.items[alias] = string(data)
	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *memoryKeyringStore) Retrieve(alias string) (types.ICredentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.items[alias]
	if !ok {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("credentials not found for alias %q", alias))
	}

	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal credential envelope: %w", err))
	}

	switch env.Type {
	case "aws":
		var c types.AWSCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal AWS credentials: %w", err))
		}
		return &c, nil
	case "oidc":
		var c types.OIDCCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal OIDC credentials: %w", err))
		}
		return &c, nil
	default:
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("unknown credential type %q", env.Type))
	}
}

// Delete deletes credentials for the given alias.
func (s *memoryKeyringStore) Delete(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[alias]; !ok {
		return errors.Join(ErrCredentialStore, fmt.Errorf("credentials not found for alias %q", alias))
	}

	delete(s.items, alias)
	return nil
}

// List returns all stored credential aliases.
func (s *memoryKeyringStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	aliases := make([]string, 0, len(s.items))
	for alias := range s.items {
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

// IsExpired checks if credentials for the given alias are expired.
func (s *memoryKeyringStore) IsExpired(alias string) (bool, error) {
	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}
	// Delegate to the credential's IsExpired implementation.
	return creds.IsExpired(), nil
}

// GetAny retrieves and unmarshals any type from the memory store.
func (s *memoryKeyringStore) GetAny(key string, dest interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.items[key]
	if !ok {
		return errors.Join(ErrCredentialStore, fmt.Errorf("data not found for key %q", key))
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal data: %w", err))
	}

	return nil
}

// SetAny marshals and stores any type in the memory store.
func (s *memoryKeyringStore) SetAny(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal data: %w", err))
	}

	s.items[key] = string(data)
	return nil
}
