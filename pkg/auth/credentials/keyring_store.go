package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	log "github.com/cloudposse/atmos/pkg/logger"
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
	// explicitRealm is the realm the store's config says was configured explicitly, so it is never
	// mistaken for the auto realm (see keyring_legacy_auto_realm.go). Empty when unknown.
	explicitRealm string
}

// Store stores credentials for the given alias within the specified realm.
func (s *keyringCredentialStore) Store(alias string, creds types.ICredentials, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.Store")()

	return s.storeEnvelope(alias, creds, realm, "")
}

// storeEnvelope persists credentials, and an optional configuration digest, in the envelope.
func (s *keyringCredentialStore) storeEnvelope(alias string, creds types.ICredentials, realm, digest string) error {
	typ, raw, err := marshalCredential(creds)
	if err != nil {
		return err
	}
	data, err := json.Marshal(&credentialEnvelope{Type: typ, Data: raw, ConfigDigest: digest, Alias: alias})
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

	creds, _, err := s.retrieveEnvelope(alias, realm)
	return creds, err
}

// retrieveEnvelope reads the envelope stored for alias within realm and returns its credentials
// and configuration digest ("" for envelopes written without one).
func (s *keyringCredentialStore) retrieveEnvelope(alias, realm string) (types.ICredentials, string, error) {
	data, err := s.kr.Get(buildKeyringKey(alias, realm))
	// Read-fallback migration: entries written before the "__" realm separator are still
	// found, but only when the old key cannot belong to another realm and alias split.
	fromLegacy := false
	if errors.Is(err, keyring.ErrNotFound) && s.legacyKeyIsUnambiguous(alias, realm) {
		data, err = s.kr.Get(legacyRealmKeyringKey(alias, realm))
		fromLegacy = err == nil
	}
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, "", errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
		}
		return nil, "", errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve credentials: %w", err))
	}

	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return nil, "", errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal credential envelope: %w", err))
	}
	// An entry stored for another alias that shares this key is not this alias's entry.
	if env.Alias != "" && env.Alias != alias {
		log.Debug("Ignoring keyring entry stored for another alias", logKeyAlias, alias, logKeyRealm, realm, "entryAlias", env.Alias)
		return nil, "", errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
	}
	creds, err := unmarshalCredential(env)
	if err != nil {
		return nil, "", err
	}
	// Only an entry that decodes is moved, so a corrupt legacy entry is never propagated.
	if fromLegacy {
		s.migrateLegacyRealmEntry(alias, realm, data)
	}
	return creds, env.ConfigDigest, nil
}

// migrateLegacyRealmEntry rewrites an entry read through the unambiguous previous-layout
// fallback under the current key and then deletes the legacy key, so later reads no longer
// depend on the fallback. It is best effort and never fails the read: on any error the legacy
// key is kept and the fallback keeps serving it. The entry is copied unchanged, and it is not
// copied when the current key already exists (written by another process since the first
// read), so fresh credentials are never overwritten by older ones.
func (s *keyringCredentialStore) migrateLegacyRealmEntry(alias, realm, data string) {
	current, legacy := buildKeyringKey(alias, realm), legacyRealmKeyringKey(alias, realm)
	exists, err := s.kr.Has(current)
	if err != nil || exists {
		log.Debug("Not migrating legacy keyring entry", logKeyAlias, alias, logKeyRealm, realm, "currentExists", exists, "error", err)
		return
	}
	if err := s.kr.Set(current, data); err != nil {
		log.Debug("Failed to migrate legacy keyring entry; keeping the legacy key", logKeyAlias, alias, logKeyRealm, realm, "error", err)
		return
	}
	if err := s.kr.Delete(legacy); err != nil {
		log.Debug("Failed to delete migrated legacy keyring entry", logKeyAlias, alias, logKeyRealm, realm, "error", err)
		return
	}
	log.Debug("Migrated legacy keyring entry to the current key layout", logKeyAlias, alias, logKeyRealm, realm)
}

// Delete deletes credentials for the given alias within the specified realm. It is idempotent.
func (s *keyringCredentialStore) Delete(alias string, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.Delete")()

	if s.keyBelongsTo(buildKeyringKey(alias, realm), alias) {
		if err := s.kr.Delete(buildKeyringKey(alias, realm)); err != nil {
			return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete credentials: %w", err))
		}
	}
	s.deleteExternalConfigDigest(alias, realm)
	// Also remove the previous-layout entry Retrieve would fall back to, so a deleted
	// credential cannot resurface. An ambiguous old key may belong to another realm and is kept.
	// The legacy key is also checked for ownership: another alias (for example "acme_admin" in the
	// empty realm) can have an entry at the same key, which is kept.
	if s.legacyKeyIsUnambiguous(alias, realm) && s.keyBelongsTo(legacyRealmKeyringKey(alias, realm), alias) {
		if err := s.kr.Delete(legacyRealmKeyringKey(alias, realm)); err != nil {
			return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete legacy credentials: %w", err))
		}
	}
	return nil
}

// keyBelongsTo reports whether the entry at key may be deleted as alias's own: it is absent,
// unreadable (kept deletable), carries no alias (written by an older version), or carries this alias. An entry stored for another alias that shares the key
// belongs to that alias and is kept.
func (s *keyringCredentialStore) keyBelongsTo(key, alias string) bool {
	data, err := s.kr.Get(key)
	if err != nil {
		return true
	}
	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return true
	}
	return env.Alias == "" || env.Alias == alias
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
	return s.filterRealmKeys(keys, realm), nil
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

// filterRealmKeys reduces a flat list of realm-scoped keyring keys to aliases. With a realm it
// returns aliases under "atmos_<realm>__" plus unambiguous previous-layout aliases under
// "atmos_<realm>_" (see legacyKeyIsUnambiguous), each once; with an empty realm it strips
// only the "atmos_" prefix (so callers see "realm__alias"), matching the historical List behavior.
func (s *keyringCredentialStore) filterRealmKeys(keys []string, realm string) []string {
	base := KeyringRealmPrefix + KeyringSeparator
	aliases := make([]string, 0, len(keys))
	if realm == "" {
		for _, key := range keys {
			if alias, ok := strings.CutPrefix(key, base); ok && alias != "" {
				aliases = append(aliases, alias)
			}
		}
		return aliases
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		alias, ok := s.realmKeyAlias(key, base, realm)
		if ok && alias != "" && !seen[alias] {
			seen[alias] = true
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

// realmKeyAlias extracts the alias from a keyring key of the given realm: from the current
// layout, or from the previous layout when that key is unambiguous.
func (s *keyringCredentialStore) realmKeyAlias(key, base, realm string) (string, bool) {
	if alias, ok := strings.CutPrefix(key, base+realm+KeyringRealmAliasSeparator); ok {
		return alias, true
	}
	alias, ok := strings.CutPrefix(key, base+realm+KeyringSeparator)
	return alias, ok && s.legacyKeyIsUnambiguous(alias, realm)
}
