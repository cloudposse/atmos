package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// systemKeyringStore implements the CredentialStore interface using the system keyring via Zalando go-keyring.
type systemKeyringStore struct{}

// newSystemKeyringStore creates a new system keyring store.
// It tests keyring availability by attempting a test operation.
// Returns an error if the system keyring is not available (e.g., no dbus in containers).
func newSystemKeyringStore() (*systemKeyringStore, error) {
	// Test keyring availability by attempting to get a non-existent key.
	// This will fail with ErrNotFound if keyring is working,
	// or with a different error (e.g., dbus error) if keyring is unavailable.
	testKey := "atmos-keyring-test"
	_, err := keyring.Get(testKey, KeyringUser)

	// If error is ErrNotFound, keyring is available (key just doesn't exist).
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// Any other error indicates keyring is not available.
		return nil, fmt.Errorf("system keyring not available: %w", err)
	}

	return &systemKeyringStore{}, nil
}

// Store stores credentials for the given alias.
func (s *systemKeyringStore) Store(alias string, creds types.ICredentials) error {
	defer perf.Track(nil, "credentials.systemKeyringStore.Store")()

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
		return fmt.Errorf("%w: %T", errors.Join(ErrCredentialStore, ErrUnsupportedCredentialType), creds)
	}
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	env := credentialEnvelope{Type: typ, Data: raw}
	data, err := json.Marshal(&env)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	if err := keyring.Set(alias, KeyringUser, string(data)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store credentials in system keyring: %w", err))
	}
	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *systemKeyringStore) Retrieve(alias string) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.systemKeyringStore.Retrieve")()

	data, err := keyring.Get(alias, KeyringUser)
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve credentials from keyring: %w", err))
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
		return nil, fmt.Errorf("%w: %q", errors.Join(ErrCredentialStore, ErrUnknownCredentialType), env.Type)
	}
}

// Delete deletes credentials for the given alias.
func (s *systemKeyringStore) Delete(alias string) error {
	defer perf.Track(nil, "credentials.systemKeyringStore.Delete")()

	if err := keyring.Delete(alias, KeyringUser); err != nil {
		// Treat "not found" as success - credential already removed.
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete credentials from keyring: %w", err))
	}

	return nil
}

// List is not supported by the system keyring store due to go-keyring library limitations.
// Returns an error combining ErrCredentialStore, ErrNotSupported, and ErrListNotSupported.
// Callers should use errors.Is to detect the not-supported condition and treat List as unsupported.
func (s *systemKeyringStore) List() ([]string, error) {
	defer perf.Track(nil, "credentials.systemKeyringStore.List")()

	// Note: go-keyring doesn't provide a list function.
	// This is a limitation - we'd need to maintain a separate index
	// or use a different storage backend for full functionality.
	return nil, errors.Join(ErrCredentialStore, ErrNotSupported, ErrListNotSupported)
}

// IsExpired checks if credentials for the given alias are expired.
func (s *systemKeyringStore) IsExpired(alias string) (bool, error) {
	defer perf.Track(nil, "credentials.systemKeyringStore.IsExpired")()

	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}
	// Delegate to the credential's IsExpired implementation.
	return creds.IsExpired(), nil
}

// GetAny retrieves and unmarshals any type from the keyring.
func (s *systemKeyringStore) GetAny(key string, dest interface{}) error {
	defer perf.Track(nil, "credentials.systemKeyringStore.GetAny")()

	data, err := keyring.Get(key, KeyringUser)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve data from keyring: %w", err))
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal data: %w", err))
	}

	return nil
}

// SetAny marshals and stores any type in the keyring.
func (s *systemKeyringStore) SetAny(key string, value interface{}) error {
	defer perf.Track(nil, "credentials.systemKeyringStore.SetAny")()

	data, err := json.Marshal(value)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal data: %w", err))
	}

	if err := keyring.Set(key, KeyringUser, string(data)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store data in keyring: %w", err))
	}

	return nil
}
