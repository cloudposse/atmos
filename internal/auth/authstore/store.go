package authstore

import (
	"encoding/json"
	"time"

	"github.com/zalando/go-keyring"
)

// Constants
const service = "atmos-auth"

// AuthCredential holds the authentication token and metadata
type AuthCredential struct {
	Method      string    `json:"method"`
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	LastUpdated time.Time `json:"last_updated"`
}

// GenericStore provides generic JSON storage to/from the OS keyring.
// Use SetAny to store any struct as JSON, and GetInto to populate a provided struct.
type GenericStore interface {
	// GetInto retrieves the JSON for alias and unmarshals it into out (which must be a pointer).
	GetInto(alias string, out any) error
	// SetAny marshals v to JSON and stores it under alias.
	SetAny(alias string, v any) error
	// Delete removes the entry for alias.
	Delete(alias string) error
}

// KeyringAuthStore implements a generic JSON store using the OS keyring
type KeyringAuthStore struct{}

func NewKeyringAuthStore() *KeyringAuthStore {
	return &KeyringAuthStore{}
}

// Delete removes the entry from the keyring
func (k *KeyringAuthStore) Delete(alias string) error {
	return keyring.Delete(service, alias)
}

// GetInto retrieves the JSON blob stored for alias and unmarshals it into out.
// out must be a pointer to the destination struct.
func (k *KeyringAuthStore) GetInto(alias string, out any) error {
	secret, err := keyring.Get(service, alias)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(secret), out)
}

// SetAny marshals v to JSON and stores it under alias.
func (k *KeyringAuthStore) SetAny(alias string, v any) error {
	bytes, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return keyring.Set(service, alias, string(bytes))
}
