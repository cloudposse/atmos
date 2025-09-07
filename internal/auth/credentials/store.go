package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrCredentialStore is the static sentinel for credential-store failures.
var ErrCredentialStore = errors.New("credential store")

// ErrNotSupported indicates an unsupported operation for this backend.
var ErrNotSupported = errors.New("not supported")

const (
	// KeyringUser is the "account" used to store credentials in the keyring
	// here we use atmos-auth to provide a consistent way to search for atmos credentials.
	KeyringUser = "atmos-auth"
)

// keyringStore implements the CredentialStore interface using the system keyring.
type keyringStore struct{}

// NewCredentialStore creates a new credential store instance.
func NewCredentialStore() types.CredentialStore {
	return &keyringStore{}
}

// Store stores credentials for the given alias.
func (s *keyringStore) Store(alias string, creds *schema.Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal credentials: %v", ErrCredentialStore, err)
	}

	if err := keyring.Set(alias, KeyringUser, string(data)); err != nil {
		return fmt.Errorf("%w: failed to store credentials in keyring: %v", ErrCredentialStore, err)
	}

	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *keyringStore) Retrieve(alias string) (*schema.Credentials, error) {
	data, err := keyring.Get(alias, KeyringUser)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve credentials from keyring: %v", ErrCredentialStore, err)
	}

	var creds schema.Credentials
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal credentials: %v", ErrCredentialStore, err)
	}

	return &creds, nil
}

// Delete deletes credentials for the given alias.
func (s *keyringStore) Delete(alias string) error {
	if err := keyring.Delete(alias, KeyringUser); err != nil {
		return fmt.Errorf("%w: failed to delete credentials from keyring: %v", ErrCredentialStore, err)
	}

	return nil
}

// List returns all stored credential aliases.
func (s *keyringStore) List() ([]string, error) {
	// Note: go-keyring doesn't provide a list function
	// This is a limitation - we'd need to maintain a separate index
	// or use a different storage backend for full functionality
	return nil, fmt.Errorf("%w: listing credentials is not supported with keyring backend", ErrCredentialStore)
}

// IsExpired checks if credentials for the given alias are expired.
func (s *keyringStore) IsExpired(alias string) (bool, error) {
	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}

	// Check AWS credentials expiration
	if creds.AWS != nil && creds.AWS.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.AWS.Expiration)
		if err != nil {
			return true, fmt.Errorf("%w: failed to parse expiration time: %v", ErrCredentialStore, err)
		}
		return time.Now().After(expTime), nil
	}

	// Check Azure credentials expiration
	if creds.Azure != nil && creds.Azure.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.Azure.Expiration)
		if err != nil {
			return true, fmt.Errorf("%w: failed to parse expiration time: %v", ErrCredentialStore, err)
		}
		return time.Now().After(expTime), nil
	}

	// Check GCP credentials expiration
	if creds.GCP != nil && creds.GCP.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.GCP.Expiration)
		if err != nil {
			return true, fmt.Errorf("%w: failed to parse expiration time: %v", ErrCredentialStore, err)
		}
		return time.Now().After(expTime), nil
	}

	// If no expiration info, assume not expired
	return false, nil
}

// GetAny retrieves and unmarshals any type from the keyring.
func (s *keyringStore) GetAny(key string, dest interface{}) error {
	data, err := keyring.Get(key, KeyringUser)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve data from keyring: %v", ErrCredentialStore, err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("%w: failed to unmarshal data: %v", ErrCredentialStore, err)
	}

	return nil
}

// SetAny marshals and stores any type in the keyring.
func (s *keyringStore) SetAny(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal data: %v", ErrCredentialStore, err)
	}

	if err := keyring.Set(key, KeyringUser, string(data)); err != nil {
		return fmt.Errorf("%w: failed to store data in keyring: %v", ErrCredentialStore, err)
	}

	return nil
}

// NewKeyringAuthStore creates a new keyring-based auth store (for backward compatibility).
func NewKeyringAuthStore() *keyringStore {
	return &keyringStore{}
}
