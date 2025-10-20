package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// ErrCredentialStore is the static sentinel for credential-store failures.
var ErrCredentialStore = errors.New("credential store")

// ErrNotSupported indicates an unsupported operation for this backend.
var ErrNotSupported = errors.New("not supported")

// ErrListNotSupported indicates that listing credentials is not supported with keyring backend.
var ErrListNotSupported = errors.New("listing credentials is not supported with keyring backend")

const (
	// KeyringUser is the "account" used to store credentials in the keyring. Here we use atmos-auth to provide a consistent way to search for atmos credentials.
	KeyringUser = "atmos-auth"
)

// keyringStore implements the CredentialStore interface using the system keyring.
type keyringStore struct{}

// NewCredentialStore creates a new credential store instance.
func NewCredentialStore() types.CredentialStore {
	return &keyringStore{}
}

// Store stores credentials for the given alias. Envelope used to persist interface credentials.
type credentialEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (s *keyringStore) Store(alias string, creds types.ICredentials) error {
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
		return fmt.Errorf("%w: unsupported credential type %T", ErrCredentialStore, creds)
	}
	if err != nil {
		return errors.Join(ErrCredentialStore, err)
	}

	env := credentialEnvelope{Type: typ, Data: raw}
	data, err := json.Marshal(&env)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal credentials: %w", ErrCredentialStore, err)
	}

	if err := keyring.Set(alias, KeyringUser, string(data)); err != nil {
		return errors.Join(ErrCredentialStore, err)
	}
	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *keyringStore) Retrieve(alias string) (types.ICredentials, error) {
	data, err := keyring.Get(alias, KeyringUser)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve credentials from keyring: %w", ErrCredentialStore, err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal credential envelope: %w", ErrCredentialStore, err)
	}

	switch env.Type {
	case "aws":
		var c types.AWSCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal AWS credentials: %w", ErrCredentialStore, err)
		}
		return &c, nil
	case "oidc":
		var c types.OIDCCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal OIDC credentials: %w", ErrCredentialStore, err)
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("%w: unknown credential type %q", ErrCredentialStore, env.Type)
	}
}

// Delete deletes credentials for the given alias.
func (s *keyringStore) Delete(alias string) error {
	if err := keyring.Delete(alias, KeyringUser); err != nil {
		return fmt.Errorf("%w: failed to delete credentials from keyring: %w", ErrCredentialStore, err)
	}

	return nil
}

// List returns all stored credential aliases.
func (s *keyringStore) List() ([]string, error) {
	// Note: go-keyring doesn't provide a list function.
	// This is a limitation - we'd need to maintain a separate index.
	// or use a different storage backend for full functionality.
	// Join both the generic store error and specific not-supported sentinel
	// so callers can detect either condition with errors.Is.
	return nil, errors.Join(ErrCredentialStore, ErrNotSupported, ErrListNotSupported)
}

// IsExpired checks if credentials for the given alias are expired.
func (s *keyringStore) IsExpired(alias string) (bool, error) {
	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}
	// Delegate to the credential's IsExpired implementation.
	return creds.IsExpired(), nil
}

// GetAny retrieves and unmarshals any type from the keyring.
func (s *keyringStore) GetAny(key string, dest interface{}) error {
	data, err := keyring.Get(key, KeyringUser)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve data from keyring: %w", ErrCredentialStore, err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("%w: failed to unmarshal data: %w", ErrCredentialStore, err)
	}

	return nil
}

// SetAny marshals and stores any type in the keyring.
func (s *keyringStore) SetAny(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal data: %w", ErrCredentialStore, err)
	}

	if err := keyring.Set(key, KeyringUser, string(data)); err != nil {
		return fmt.Errorf("%w: failed to store data in keyring: %w", ErrCredentialStore, err)
	}

	return nil
}

// NewKeyringAuthStore creates a new keyring-based auth store (for backward compatibility).
func NewKeyringAuthStore() *keyringStore {
	return &keyringStore{}
}
