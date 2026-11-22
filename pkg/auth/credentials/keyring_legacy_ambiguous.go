package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Every keyring-backed store (system, file, memory, noop) implements AmbiguousLegacyRealmStore.
var _ AmbiguousLegacyRealmStore = (*keyringCredentialStore)(nil)

// LegacyRealmOwner is a realm and alias pair whose previous-layout keyring key
// ("atmos_<realm>_<alias>") is a given key.
type LegacyRealmOwner struct {
	// Realm is the credential realm.
	Realm string
	// Alias is the identity or provider name.
	Alias string
}

// AmbiguousLegacyRealmEntry describes a previous-layout keyring entry that may hold the
// credentials of an alias in a realm but could also have been written for another realm and
// alias split of the same key. It never carries the raw entry, only the decoded credentials.
type AmbiguousLegacyRealmEntry struct {
	// Key is the previous-layout keyring key, for example "atmos_acme_prod_admin".
	Key string
	// Credentials are the decoded credentials stored under Key.
	Credentials types.ICredentials
	// OtherOwners lists the other realm and alias pairs that older versions also stored
	// under Key, so a caller can tell the user who else the entry could belong to.
	OtherOwners []LegacyRealmOwner
}

// AmbiguousLegacyRealmStore is implemented by credential stores that can find and adopt
// previous-layout keyring entries whose key is ambiguous. Retrieve never reads such entries
// (see legacyKeyIsUnambiguous) because they might belong to another realm; a caller that
// can ask the user whether the entry is theirs uses this interface instead.
type AmbiguousLegacyRealmStore interface {
	// FindAmbiguousLegacyRealmEntry returns the ambiguous previous-layout entry for alias in
	// realm, or nil when there is nothing to adopt: the current key exists, the legacy key is
	// unambiguous (Retrieve migrates those itself), absent, or does not decode.
	FindAmbiguousLegacyRealmEntry(alias, realm string) (*AmbiguousLegacyRealmEntry, error)
	// AdoptAmbiguousLegacyRealmEntry copies the ambiguous previous-layout entry to the current
	// key of alias in realm, unless the current key already exists, and then deletes the legacy
	// key. Call it only after the user confirmed the entry belongs to alias in realm.
	AdoptAmbiguousLegacyRealmEntry(alias, realm string) error
}

// FindAmbiguousLegacyRealmEntry implements AmbiguousLegacyRealmStore.
func (s *keyringCredentialStore) FindAmbiguousLegacyRealmEntry(alias, realm string) (*AmbiguousLegacyRealmEntry, error) {
	defer perf.Track(nil, "credentials.keyringCredentialStore.FindAmbiguousLegacyRealmEntry")()

	legacy, ok := s.ambiguousLegacyRealmKey(alias, realm)
	if !ok {
		return nil, nil
	}
	exists, err := s.kr.Has(buildKeyringKey(alias, realm))
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to probe credentials: %w", err))
	}
	if exists {
		return nil, nil
	}
	_, creds, err := s.readLegacyEntry(legacy)
	if err != nil || creds == nil {
		return nil, err
	}
	return &AmbiguousLegacyRealmEntry{Key: legacy, Credentials: creds, OtherOwners: otherLegacyRealmOwners(alias, realm)}, nil
}

// AdoptAmbiguousLegacyRealmEntry implements AmbiguousLegacyRealmStore.
//
// The legacy key is deleted after the copy because the user confirmed the entry is theirs:
// keeping it would leave a second copy of the secret that Delete never removes (it only
// removes unambiguous legacy keys), and would make the other realm and alias split prompt
// about an entry that is now known not to be its own. A failed delete is logged and ignored,
// because the copy already succeeded and later reads use the current key.
func (s *keyringCredentialStore) AdoptAmbiguousLegacyRealmEntry(alias, realm string) error {
	defer perf.Track(nil, "credentials.keyringCredentialStore.AdoptAmbiguousLegacyRealmEntry")()

	legacy, ok := s.ambiguousLegacyRealmKey(alias, realm)
	if !ok {
		return errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
	}
	data, creds, err := s.readLegacyEntry(legacy)
	if err != nil {
		return err
	}
	if creds == nil {
		return errors.Join(ErrCredentialStore, ErrCredentialsNotFound)
	}
	current := buildKeyringKey(alias, realm)
	exists, err := s.kr.Has(current)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to probe credentials: %w", err))
	}
	// Credentials written under the current key since the prompt are newer; never overwrite them.
	if !exists {
		if err := s.kr.Set(current, data); err != nil {
			return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store credentials: %w", err))
		}
	}
	if err := s.kr.Delete(legacy); err != nil {
		log.Debug("Failed to delete adopted legacy keyring entry", "alias", alias, "realm", realm, "error", err)
		return nil
	}
	log.Debug("Adopted ambiguous legacy keyring entry after confirmation", "alias", alias, "realm", realm)
	return nil
}

// readLegacyEntry returns the raw entry under key and its decoded credentials. A missing or
// undecodable entry returns nil credentials and no error: it is not offered for adoption, and
// a corrupt entry is never propagated. Other keyring failures are returned.
func (s *keyringCredentialStore) readLegacyEntry(key string) (string, types.ICredentials, error) {
	data, err := s.kr.Get(key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve credentials: %w", err))
	}
	var env credentialEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		log.Debug("Ignoring undecodable legacy keyring entry", "error", err)
		return "", nil, nil
	}
	creds, err := unmarshalCredential(env)
	if err != nil {
		log.Debug("Ignoring undecodable legacy keyring entry", "error", err)
		return "", nil, nil
	}
	return data, creds, nil
}

// ambiguousLegacyRealmKey returns the previous-layout key of alias in realm when it is
// ambiguous and may be offered for adoption. It reports false when there is no realm, when the
// key is unambiguous (Retrieve migrates it, see legacyKeyIsUnambiguous), and when the key contains "__": such a key is also
// a current-layout key of another realm (realm "acme" + alias "x__y" is "atmos_acme_x__y",
// the current key of realm "acme_x" + alias "y"), so adopting and deleting it could break a
// live entry.
func (s *keyringCredentialStore) ambiguousLegacyRealmKey(alias, realm string) (string, bool) {
	if realm == "" || alias == "" || s.legacyKeyIsUnambiguous(alias, realm) {
		return "", false
	}
	key := legacyRealmKeyringKey(alias, realm)
	rest := strings.TrimPrefix(key, KeyringRealmPrefix+KeyringSeparator)
	if strings.Contains(rest, KeyringRealmAliasSeparator) {
		return "", false
	}
	return key, true
}

// otherLegacyRealmOwners lists every other split of "<realm>_<alias>" into a valid realm and a
// non-empty alias, that is every other pair whose previous-layout key equals this pair's key.
func otherLegacyRealmOwners(alias, realmValue string) []LegacyRealmOwner {
	joined := realmValue + KeyringSeparator + alias
	var owners []LegacyRealmOwner
	for i := 0; i < len(joined); i++ {
		if joined[i] != KeyringSeparator[0] {
			continue
		}
		// Index len(realmValue) is this pair's own split.
		candidateRealm, candidateAlias := joined[:i], joined[i+1:]
		if i == len(realmValue) || candidateAlias == "" || realm.Validate(candidateRealm) != nil {
			continue
		}
		owners = append(owners, LegacyRealmOwner{Realm: candidateRealm, Alias: candidateAlias})
	}
	return owners
}
