package credentials

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "atmos-auth"
)

// keyringStore implements the CredentialStore interface using the system keyring
type keyringStore struct{}

// NewKeyringCredentialStore creates a new keyring-based credential store
func NewKeyringCredentialStore() auth.CredentialStore {
	return &keyringStore{}
}

// Store stores credentials for the given alias
func (s *keyringStore) Store(alias string, creds *schema.Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := keyring.Set(keyringService, alias, string(data)); err != nil {
		return fmt.Errorf("failed to store credentials in keyring: %w", err)
	}

	return nil
}

// Retrieve retrieves credentials for the given alias
func (s *keyringStore) Retrieve(alias string) (*schema.Credentials, error) {
	data, err := keyring.Get(keyringService, alias)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials from keyring: %w", err)
	}

	var creds schema.Credentials
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return &creds, nil
}

// Delete deletes credentials for the given alias
func (s *keyringStore) Delete(alias string) error {
	if err := keyring.Delete(keyringService, alias); err != nil {
		return fmt.Errorf("failed to delete credentials from keyring: %w", err)
	}

	return nil
}

// List returns all stored credential aliases
func (s *keyringStore) List() ([]string, error) {
	// Note: go-keyring doesn't provide a list function
	// This is a limitation - we'd need to maintain a separate index
	// or use a different storage backend for full functionality
	return nil, fmt.Errorf("listing credentials is not supported with keyring backend")
}

// IsExpired checks if credentials for the given alias are expired
func (s *keyringStore) IsExpired(alias string) (bool, error) {
	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}

	// Check AWS credentials expiration
	if creds.AWS != nil && creds.AWS.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.AWS.Expiration)
		if err != nil {
			return true, fmt.Errorf("failed to parse expiration time: %w", err)
		}
		return time.Now().After(expTime), nil
	}

	// Check Azure credentials expiration
	if creds.Azure != nil && creds.Azure.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.Azure.Expiration)
		if err != nil {
			return true, fmt.Errorf("failed to parse expiration time: %w", err)
		}
		return time.Now().After(expTime), nil
	}

	// Check GCP credentials expiration
	if creds.GCP != nil && creds.GCP.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.GCP.Expiration)
		if err != nil {
			return true, fmt.Errorf("failed to parse expiration time: %w", err)
		}
		return time.Now().After(expTime), nil
	}

	// If no expiration info, assume not expired
	return false, nil
}
