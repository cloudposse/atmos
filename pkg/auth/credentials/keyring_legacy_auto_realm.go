package credentials

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The previous key layout "atmos_<realm>_<alias>" is ambiguous when the realm or the alias
// contains "_" (see legacyRealmKeyringKey). Under the realm Atmos derives from the atmos.yaml
// path when no realm is configured (16 lowercase hex characters, never containing "_"), the
// only other realm and alias splits of such a key are realms "<hash>_<prefix>": a configured
// realm that begins with the 16-character hash of this repository's atmos.yaml path. Nobody
// configures one, so under the auto realm the legacy key belongs to this alias in practice and is
// read, migrated, and deleted silently, like the key of an alias without "_" (regression A2 of
// the 1.235.0 regression test). Realms that were configured explicitly keep the confirmation.
//
// The store only receives the realm string, and the realm source does not reach every store:
// MergeComponentAuthConfig drops schema.AuthConfig.RealmSource (it is not serialized), and some
// callers build a store without a config. The auto realm is therefore recognized by its exact
// format, and a realm the store's config says was set explicitly (atmos.yaml auth.realm or
// ATMOS_AUTH_REALM) is excluded even when it has that format.

// explicitRealmSources are the schema.AuthConfig.RealmSource values of a configured realm.
var explicitRealmSources = map[string]bool{realm.SourceConfig: true, realm.SourceEnv: true}

// explicitRealmOf returns the realm authConfig says was configured explicitly, or "".
func explicitRealmOf(authConfig *schema.AuthConfig) string {
	if authConfig == nil || !explicitRealmSources[authConfig.RealmSource] {
		return ""
	}
	return authConfig.Realm
}

// isAutoRealm reports whether realmValue is the realm Atmos derived from the atmos.yaml path.
func (s *keyringCredentialStore) isAutoRealm(realmValue string) bool {
	return realm.HasAutoValueFormat(realmValue) && realmValue != s.explicitRealm
}

// legacyKeyIsUnambiguous reports whether the previous-layout key of alias in realmValue is
// treated as this pair's own key: read through the fallback, migrated, deleted with it, and
// listed. That holds when neither part contains "_" (legacyRealmKeyIsUnambiguous), and under the
// auto realm for every alias whose legacy key contains no "__": such a key is also the
// current-layout key of another alias ("atmos_<hash>__x" for alias "_x" is the current key of
// alias "x"), so it is never touched as a legacy key.
func (s *keyringCredentialStore) legacyKeyIsUnambiguous(alias, realmValue string) bool {
	if legacyRealmKeyIsUnambiguous(alias, realmValue) {
		return true
	}
	return alias != "" && s.isAutoRealm(realmValue) &&
		!strings.Contains(KeyringSeparator+alias, KeyringRealmAliasSeparator)
}

// setExplicitRealm records the explicitly configured realm of authConfig on store, when store is
// keyring backed, so that realm is never mistaken for the auto realm.
func setExplicitRealm(store any, authConfig *schema.AuthConfig) {
	if receiver, ok := store.(interface{ setExplicitRealm(string) }); ok {
		receiver.setExplicitRealm(explicitRealmOf(authConfig))
	}
}

// setExplicitRealm implements the receiver used by the package-level setExplicitRealm.
func (s *keyringCredentialStore) setExplicitRealm(value string) {
	s.explicitRealm = value
}
