package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema fields these tests rely on.
var _ = schema.AuthConfig{Realm: autoTestRealm, RealmSource: realm.SourceConfig}

// autoTestRealm has the format of the realm Atmos derives from the atmos.yaml path when no
// realm is configured (pkg/config/auth_realm.go): 16 lowercase hexadecimal characters.
const autoTestRealm = "3ed941021e53d074"

// seedLegacyAutoRealmKeys stores long-lived AWS keys under the previous-layout key
// "atmos_<realm>_<alias>", as Atmos v1.234.1 did, and returns the raw entry.
func seedLegacyAutoRealmKeys(t *testing.T, store *keyringCredentialStore, alias, realmValue string) (*types.AWSCredentials, string) {
	t.Helper()
	keys := &types.AWSCredentials{AccessKeyID: "AKIALEGACYAUTO", SecretAccessKey: "legacy-secret"}
	require.NoError(t, store.Store(alias, keys, realmValue))
	raw, err := store.kr.Get(buildKeyringKey(alias, realmValue))
	require.NoError(t, err)
	require.NoError(t, store.kr.Delete(buildKeyringKey(alias, realmValue)))
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey(alias, realmValue), raw))
	return keys, raw
}

// TestKeyringStore_AutoRealmLegacyKeyWithUnderscoreAliasIsMigrated reproduces regression A2 of
// the 1.235.0 regression test: under the realm derived from the atmos.yaml path (which never
// contains "_"), the previous-layout key of an alias with "_" could only also belong to a realm
// "<hash>_<prefix>", which no user configures. The entry must be read and migrated like the
// entry of an alias without "_", instead of being refused as ambiguous.
func TestKeyringStore_AutoRealmLegacyKeyWithUnderscoreAliasIsMigrated(t *testing.T) {
	store := newMemoryKeyringStore()
	keys, raw := seedLegacyAutoRealmKeys(t, store, "prod_admin", autoTestRealm)

	aliases, err := store.List(autoTestRealm)
	require.NoError(t, err)
	assert.Equal(t, []string{"prod_admin"}, aliases, "List must report the migratable legacy alias")

	entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", autoTestRealm)
	require.NoError(t, err)
	assert.Nil(t, entry, "a legacy key under the auto realm is not offered for confirmation")

	got, err := store.Retrieve("prod_admin", autoTestRealm)
	require.NoError(t, err, "the legacy entry must be read without confirmation")
	assert.Equal(t, keys, got)

	migrated, err := store.kr.Get(buildKeyringKey("prod_admin", autoTestRealm))
	require.NoError(t, err, "the legacy entry must be rewritten under the current key")
	assert.Equal(t, raw, migrated)
	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", autoTestRealm))
	require.ErrorIs(t, err, keyring.ErrNotFound, "the legacy key must be removed after the rewrite")
}

// TestKeyringStore_AutoRealmLegacyKeyIsDeleted verifies Delete removes the previous-layout key
// of an alias with "_" under the auto realm, so deleted credentials cannot resurface.
func TestKeyringStore_AutoRealmLegacyKeyIsDeleted(t *testing.T) {
	store := newMemoryKeyringStore()
	seedLegacyAutoRealmKeys(t, store, "prod_admin", autoTestRealm)

	require.NoError(t, store.Delete("prod_admin", autoTestRealm))

	_, err := store.kr.Get(legacyRealmKeyringKey("prod_admin", autoTestRealm))
	require.ErrorIs(t, err, keyring.ErrNotFound)
	_, err = store.Retrieve("prod_admin", autoTestRealm)
	require.ErrorIs(t, err, ErrCredentialsNotFound)
}

// TestKeyringStore_ExplicitRealmLegacyKeyStaysAmbiguous is the negative path: realms that were
// configured explicitly keep the ambiguity handling, including a configured realm that happens
// to have the auto realm's format, and so do aliases whose legacy key contains "__".
func TestKeyringStore_ExplicitRealmLegacyKeyStaysAmbiguous(t *testing.T) {
	assertAmbiguous := func(t *testing.T, store *keyringCredentialStore, alias, realmValue string) {
		t.Helper()
		_, raw := seedLegacyAutoRealmKeys(t, store, alias, realmValue)

		_, err := store.Retrieve(alias, realmValue)
		require.ErrorIs(t, err, ErrCredentialsNotFound, "an ambiguous legacy key is never read silently")
		aliases, err := store.List(realmValue)
		require.NoError(t, err)
		assert.Empty(t, aliases)
		require.NoError(t, store.Delete(alias, realmValue))
		got, err := store.kr.Get(legacyRealmKeyringKey(alias, realmValue))
		require.NoError(t, err, "Delete keeps an ambiguous legacy key")
		assert.Equal(t, raw, got)
	}

	t.Run("explicit realm without the auto format", func(t *testing.T) {
		store := newMemoryKeyringStore()
		assertAmbiguous(t, store, "prod_admin", "acme")
		entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.NoError(t, err)
		require.NotNil(t, entry, "the entry is still offered for confirmation")
	})

	for _, source := range []string{realm.SourceConfig, realm.SourceEnv} {
		t.Run("configured realm with the auto format from "+source, func(t *testing.T) {
			t.Setenv("ATMOS_KEYRING_TYPE", "memory")
			store, ok := NewCredentialStoreWithConfig(&schema.AuthConfig{Realm: autoTestRealm, RealmSource: source}).(*keyringCredentialStore)
			require.True(t, ok)
			assertAmbiguous(t, store, "prod_admin", autoTestRealm)
			entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", autoTestRealm)
			require.NoError(t, err)
			require.NotNil(t, entry, "the entry is still offered for confirmation")
		})
	}

	t.Run("auto realm alias containing a double underscore", func(t *testing.T) {
		// "atmos_<hash>_x__y" is also the current key of realm "<hash>_x" + alias "y".
		assertAmbiguous(t, newMemoryKeyringStore(), "x__y", autoTestRealm)
	})
}

// TestKeyringStore_AutoRealmNeverTouchesAnotherAliasCurrentKey verifies that the legacy key of an
// alias starting with "_" under the auto realm, which equals the current key of the alias without
// it ("atmos_<hash>__x"), is neither read nor deleted as a legacy key.
func TestKeyringStore_AutoRealmNeverTouchesAnotherAliasCurrentKey(t *testing.T) {
	store := newMemoryKeyringStore()
	keys := &types.AWSCredentials{AccessKeyID: "AKIAOTHER", SecretAccessKey: "other-secret"}
	require.NoError(t, store.Store("x", keys, autoTestRealm))
	require.Equal(t, buildKeyringKey("x", autoTestRealm), legacyRealmKeyringKey("_x", autoTestRealm))

	_, err := store.Retrieve("_x", autoTestRealm)
	require.ErrorIs(t, err, ErrCredentialsNotFound)
	require.NoError(t, store.Delete("_x", autoTestRealm))

	got, err := store.Retrieve("x", autoTestRealm)
	require.NoError(t, err, "the current entry of alias x must survive")
	assert.Equal(t, keys, got)
}

// TestKeyringStore_AutoRealmFromConfigPathIsNotExplicit verifies the realm Atmos derived from the
// atmos.yaml path (source "config-path") is not mistaken for an explicitly configured one.
func TestKeyringStore_AutoRealmFromConfigPathIsNotExplicit(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	store, ok := NewCredentialStoreWithConfig(&schema.AuthConfig{Realm: autoTestRealm, RealmSource: "config-path"}).(*keyringCredentialStore)
	require.True(t, ok)
	keys, _ := seedLegacyAutoRealmKeys(t, store, "prod_admin", autoTestRealm)

	got, err := store.Retrieve("prod_admin", autoTestRealm)
	require.NoError(t, err)
	assert.Equal(t, keys, got)
}
