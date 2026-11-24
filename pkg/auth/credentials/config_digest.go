package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// KeyringConfigDigestPrefix prefixes the keys of configuration digest records kept for
// credentials an identity persists outside this store (for example AWS session credentials in
// the identity's credentials file). It differs from KeyringRealmPrefix + KeyringSeparator, so
// List never reports these records as credential aliases.
const KeyringConfigDigestPrefix = "atmos-config-digest"

// Every keyring-backed store (system, file, memory, noop) implements ConfigDigestStore;
// SupportsConfigDigests reports whether a backend actually persists the digests.
var _ ConfigDigestStore = (*keyringCredentialStore)(nil)

// ExternalConfigDigest records which effective configuration produced credentials that an
// identity persisted outside the credential store. Expiration and CredentialID bind the record to
// that exact credential set: credentials rewritten by anything else (an older Atmos, another
// tool, another principal) carry a different expiration or access key ID and no longer match.
// No field is secret.
type ExternalConfigDigest struct {
	// Digest is the SHA-256 of the chain step's non-secret effective configuration.
	Digest string `json:"digest"`
	// Expiration is the credentials' expiration in RFC 3339 UTC, or empty when they have none.
	Expiration string `json:"expiration,omitempty"`
	// CredentialID is the non-secret identifier of the credential set (the AWS access key ID of
	// session credentials), or empty for credentials without one.
	CredentialID string `json:"credential_id,omitempty"`
}

// ConfigDigestStore is implemented by credential stores that can persist a digest of the
// effective configuration that produced a credential, so a later process can refuse to reuse
// credentials written for a different configuration under the same alias. The digest is
// computed by the caller over non-secret fields only; the store treats it as opaque.
type ConfigDigestStore interface {
	// SupportsConfigDigests reports whether digests written now can be read back later.
	SupportsConfigDigests() bool
	// StoreWithConfigDigest stores credentials like Store, together with digest.
	StoreWithConfigDigest(alias string, creds types.ICredentials, realm, digest string) error
	// RetrieveWithConfigDigest retrieves credentials like Retrieve, together with the digest
	// they were stored with. Entries written without one (by older versions) return "".
	RetrieveWithConfigDigest(alias, realm string) (types.ICredentials, string, error)
	// StoreExternalConfigDigest records the digest of credentials persisted outside the store.
	StoreExternalConfigDigest(alias, realm string, record ExternalConfigDigest) error
	// RetrieveExternalConfigDigest returns the record written by StoreExternalConfigDigest.
	RetrieveExternalConfigDigest(alias, realm string) (ExternalConfigDigest, error)
	// DeleteExternalConfigDigest removes the record written by StoreExternalConfigDigest. It is
	// idempotent. Delete also removes it; logout calls this directly because it removes the
	// credentials the record describes even when it keeps the keyring entry.
	DeleteExternalConfigDigest(alias, realm string) error
}

// buildConfigDigestKey creates the realm-scoped key of an external configuration digest record,
// using the same realm and alias layout as buildKeyringKey.
func buildConfigDigestKey(alias, realm string) string {
	if realm != "" {
		return KeyringConfigDigestPrefix + KeyringSeparator + realm + KeyringRealmAliasSeparator + alias
	}
	return KeyringConfigDigestPrefix + KeyringSeparator + alias
}

// SupportsConfigDigests reports whether this backend persists what it stores. The noop backend
// stores nothing, so digests written through it can never be read back.
func (s *keyringCredentialStore) SupportsConfigDigests() bool {
	return s.storeType != types.CredentialStoreTypeNoop
}

// StoreWithConfigDigest stores credentials for alias within realm together with digest.
func (s *keyringCredentialStore) StoreWithConfigDigest(alias string, creds types.ICredentials, realm, digest string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.StoreWithConfigDigest")()

	return s.storeEnvelope(alias, creds, realm, digest)
}

// RetrieveWithConfigDigest retrieves credentials for alias within realm and their digest.
func (s *keyringCredentialStore) RetrieveWithConfigDigest(alias, realm string) (types.ICredentials, string, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.RetrieveWithConfigDigest")()

	return s.retrieveEnvelope(alias, realm)
}

// StoreExternalConfigDigest records the configuration digest of credentials alias persisted
// outside this store.
func (s *keyringCredentialStore) StoreExternalConfigDigest(alias, realm string, record ExternalConfigDigest) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.StoreExternalConfigDigest")()

	// A struct of two strings always marshals.
	data, _ := json.Marshal(record)
	if err := s.kr.Set(buildConfigDigestKey(alias, realm), string(data)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store configuration digest: %w", err))
	}
	return nil
}

// RetrieveExternalConfigDigest returns the configuration digest recorded for alias within realm.
func (s *keyringCredentialStore) RetrieveExternalConfigDigest(alias, realm string) (ExternalConfigDigest, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.RetrieveExternalConfigDigest")()

	var record ExternalConfigDigest
	data, err := s.kr.Get(buildConfigDigestKey(alias, realm))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return record, errors.Join(ErrCredentialStore, ErrDataNotFound)
		}
		return record, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve configuration digest: %w", err))
	}
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return ExternalConfigDigest{}, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal configuration digest: %w", err))
	}
	return record, nil
}

// DeleteExternalConfigDigest removes the configuration digest record of alias within realm.
func (s *keyringCredentialStore) DeleteExternalConfigDigest(alias, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.DeleteExternalConfigDigest")()

	if err := s.kr.Delete(buildConfigDigestKey(alias, realm)); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete configuration digest: %w", err))
	}
	return nil
}

// deleteExternalConfigDigest removes the configuration digest record of alias within realm. It
// is best effort: a leftover record is harmless, because it only describes credentials and
// stops matching once they are rewritten (see ExternalConfigDigest.Expiration).
func (s *keyringCredentialStore) deleteExternalConfigDigest(alias, realm string) {
	if err := s.DeleteExternalConfigDigest(alias, realm); err != nil {
		log.Debug("Failed to delete configuration digest record", logKeyAlias, alias, logKeyRealm, realm, "error", err)
	}
}
