package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
)

// keyringCredentialStore implements types.CredentialStore over a generic pkg/keyring backend.
// It owns the credential-specific layer — realm-scoped keys (buildKeyringKey), the typed
// credentialEnvelope, and expiry checks — and delegates raw key->value storage to the keyring.
// The four backends (system/file/memory/noop) differ only by the keyring.Keyring they wrap and
// the Type() string they report.
type keyringCredentialStore struct {
	kr        keyring.Keyring
	storeType string
}

// Store stores credentials for the given alias within the specified realm.
func (s *keyringCredentialStore) Store(alias string, creds types.ICredentials, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.Store")()

	typ, raw, err := marshalCredential(creds)
	if err != nil {
		return err
	}
	data, err := json.Marshal(&credentialEnvelope{Type: typ, Data: raw})
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}
	if err := s.kr.Set(buildKeyringKey(alias, realm), string(data)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store credentials: %w", err))
	}
	return nil
}

// Retrieve retrieves credentials for the given alias within the specified realm.
func (s *keyringCredentialStore) Retrieve(alias string, realm string) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.Retrieve")()

	data, err := s.kr.Get(buildKeyringKey(alias, realm))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
		}
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve credentials: %w", err))
	}

	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal credential envelope: %w", err))
	}
	return unmarshalCredential(env)
}

// Delete deletes credentials for the given alias within the specified realm. It is idempotent.
func (s *keyringCredentialStore) Delete(alias string, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.Delete")()

	if err := s.kr.Delete(buildKeyringKey(alias, realm)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete credentials: %w", err))
	}
	return nil
}

// List returns all stored credential aliases within the specified realm. Backends that cannot
// enumerate (the system keyring) return an error satisfying ErrListNotSupported.
func (s *keyringCredentialStore) List(realm string) ([]string, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.List")()

	keys, err := s.kr.List()
	if err != nil {
		if errors.Is(err, keyring.ErrListNotSupported) {
			return nil, errors.Join(ErrCredentialStore, ErrNotSupported, ErrListNotSupported)
		}
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to list credentials: %w", err))
	}
	return filterRealmKeys(keys, realm), nil
}

// IsExpired checks if credentials for the given alias are expired within the specified realm.
func (s *keyringCredentialStore) IsExpired(alias string, realm string) (bool, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.IsExpired")()

	creds, err := s.Retrieve(alias, realm)
	if err != nil {
		return true, err
	}
	return creds.IsExpired(), nil
}

// Type returns the credential-store type string (e.g. "system-keyring", "file", "memory", "noop").
func (s *keyringCredentialStore) Type() string {
	return s.storeType
}

// GetAny retrieves and JSON-unmarshals an arbitrary value stored under a raw (non-realm-scoped) key.
func (s *keyringCredentialStore) GetAny(key string, dest interface{}) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.GetAny")()

	data, err := s.kr.Get(key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
		}
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve data: %w", err))
	}
	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal data: %w", err))
	}
	return nil
}

// SetAny JSON-marshals and stores an arbitrary value under a raw (non-realm-scoped) key.
func (s *keyringCredentialStore) SetAny(key string, value interface{}) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.SetAny")()

	data, err := json.Marshal(value)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal data: %w", err))
	}
	if err := s.kr.Set(key, string(data)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store data: %w", err))
	}
	return nil
}

// marshalCredential maps a concrete credential type to its envelope type tag and JSON payload.
func marshalCredential(creds types.ICredentials) (string, []byte, error) {
	var typ string
	switch creds.(type) {
	case *types.AWSCredentials:
		typ = "aws"
	case *types.GCPCredentials:
		typ = "gcp"
	case *types.OIDCCredentials:
		typ = "oidc"
	case *types.ProCredentials:
		typ = "atmos-pro"
	case *mock.Credentials:
		typ = "mock"
	default:
		return "", nil, fmt.Errorf("%w: %T", errors.Join(ErrCredentialStore, ErrUnsupportedCredentialType), creds)
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return "", nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}
	return typ, raw, nil
}

// unmarshalCredential reconstructs the concrete credential type named by the envelope.
func unmarshalCredential(env credentialEnvelope) (types.ICredentials, error) {
	switch env.Type {
	case "aws":
		return decodeCredential(env.Data, &types.AWSCredentials{}, "AWS")
	case "gcp":
		return decodeCredential(env.Data, &types.GCPCredentials{}, "GCP")
	case "oidc":
		return decodeCredential(env.Data, &types.OIDCCredentials{}, "OIDC")
	case "atmos-pro":
		return decodeCredential(env.Data, &types.ProCredentials{}, "Atmos Pro")
	case "mock":
		return decodeCredential(env.Data, &mock.Credentials{}, "mock")
	default:
		return nil, fmt.Errorf("%w: %q", errors.Join(ErrCredentialStore, ErrUnknownCredentialType), env.Type)
	}
}

// decodeCredential JSON-unmarshals the envelope payload into the given concrete credential.
func decodeCredential[T types.ICredentials](data []byte, into T, name string) (types.ICredentials, error) {
	if err := json.Unmarshal(data, into); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal %s credentials: %w", name, err))
	}
	return into, nil
}

// filterRealmKeys reduces a flat list of realm-scoped keyring keys to aliases. With a realm it
// returns aliases under "atmos_<realm>_"; with an empty realm it strips only the "atmos_" prefix
// (so callers see "realm_alias"), matching the historical List behavior.
func filterRealmKeys(keys []string, realm string) []string {
	prefix := KeyringRealmPrefix + KeyringSeparator
	if realm != "" {
		prefix += realm + KeyringSeparator
	}
	aliases := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			aliases = append(aliases, key[len(prefix):])
		}
	}
	return aliases
}
