package authstore

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "atmos-auth"
)

// KeyringAuthStore provides a simple keyring-based storage for backward compatibility
type KeyringAuthStore struct{}

// NewKeyringAuthStore creates a new keyring auth store
func NewKeyringAuthStore() *KeyringAuthStore {
	return &KeyringAuthStore{}
}

// SetAny stores any serializable data in the keyring
func (s *KeyringAuthStore) SetAny(key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := keyring.Set(keyringService, key, string(jsonData)); err != nil {
		return fmt.Errorf("failed to store data in keyring: %w", err)
	}

	return nil
}

// GetAny retrieves and unmarshals data from the keyring
func (s *KeyringAuthStore) GetAny(key string, target interface{}) error {
	data, err := keyring.Get(keyringService, key)
	if err != nil {
		return fmt.Errorf("failed to retrieve data from keyring: %w", err)
	}

	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

// Delete removes data from the keyring
func (s *KeyringAuthStore) Delete(key string) error {
	if err := keyring.Delete(keyringService, key); err != nil {
		return fmt.Errorf("failed to delete data from keyring: %w", err)
	}

	return nil
}
