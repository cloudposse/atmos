package credentials

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

// TestKeyringKeys_DoNotCollideAcrossRealms reproduces #3720 finding 3.4: realm "acme"
// with identity "prod_admin" and realm "acme_prod" with identity "admin" both mapped to
// "atmos_acme_prod_admin", so one realm read (and overwrote) the other's credentials.
func TestKeyringKeys_DoNotCollideAcrossRealms(t *testing.T) {
	assert.NotEqual(t, buildKeyringKey("prod_admin", "acme"), buildKeyringKey("admin", "acme_prod"))

	store := newMemoryKeyringStore()
	require.NoError(t, store.Store("prod_admin", &types.OIDCCredentials{Token: "acme-token"}, "acme"))

	_, err := store.Retrieve("admin", "acme_prod")
	require.ErrorIs(t, err, ErrCredentialsNotFound, "another realm's credentials must never be returned")

	require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "acme-prod-token"}, "acme_prod"))
	got, err := store.Retrieve("prod_admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "acme-token", got.(*types.OIDCCredentials).Token, "storing one realm must not overwrite another")
}

// TestKeyringKeys_EmptyRealmNeverCollidesWithRealmScoped verifies the legacy empty-realm
// key space ("atmos_<alias>") and the realm-scoped one stay disjoint for aliases
// without a double underscore.
func TestKeyringKeys_EmptyRealmNeverCollidesWithRealmScoped(t *testing.T) {
	assert.NotEqual(t, buildKeyringKey("prod_admin", ""), buildKeyringKey("admin", "prod"))
	assert.NotEqual(t, buildKeyringKey("acme_prod_admin", ""), buildKeyringKey("prod_admin", "acme"))
}

// TestKeyringStore_ReadsUnambiguousLegacyRealmKey verifies the read-fallback migration:
// a credential written under the previous "atmos_<realm>_<alias>" layout is still found
// when neither the realm nor the alias contains an underscore, so that the old key
// cannot belong to a different realm and alias split.
func TestKeyringStore_ReadsUnambiguousLegacyRealmKey(t *testing.T) {
	store := newMemoryKeyringStore()
	writeLegacyRealmEntry(t, store, "admin", "acme", "legacy-token")

	got, err := store.Retrieve("admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "legacy-token", got.(*types.OIDCCredentials).Token)

	// A newer entry in the current layout takes precedence over the legacy one.
	require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "current-token"}, "acme"))
	got, err = store.Retrieve("admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "current-token", got.(*types.OIDCCredentials).Token)

	// Delete removes both layouts so the legacy entry cannot resurface.
	require.NoError(t, store.Delete("admin", "acme"))
	_, err = store.Retrieve("admin", "acme")
	require.ErrorIs(t, err, ErrCredentialsNotFound)
	_, err = store.kr.Get(legacyRealmKeyringKey("admin", "acme"))
	require.Error(t, err, "the legacy entry must be deleted too")
}

// TestKeyringStore_IgnoresAmbiguousLegacyRealmKey verifies the fallback never reads an
// old-layout key that another realm and alias split could have written.
func TestKeyringStore_IgnoresAmbiguousLegacyRealmKey(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		realm string
	}{
		{name: "alias with underscore", alias: "prod_admin", realm: "acme"},
		{name: "realm with underscore", alias: "admin", realm: "acme_prod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryKeyringStore()
			writeLegacyRealmEntry(t, store, tt.alias, tt.realm, "maybe-another-realm")

			_, err := store.Retrieve(tt.alias, tt.realm)
			require.ErrorIs(t, err, ErrCredentialsNotFound)

			// Delete must not remove an ambiguous legacy key either.
			require.NoError(t, store.Delete(tt.alias, tt.realm))
			_, err = store.kr.Get(legacyRealmKeyringKey(tt.alias, tt.realm))
			require.NoError(t, err, "an ambiguous legacy entry may belong to another realm and must be kept")
		})
	}
}

// TestKeyringStore_LegacyFallbackPropagatesBackendErrors verifies a non-not-found error
// from the legacy probe is not masked as "not found".
func TestKeyringStore_LegacyFallbackPropagatesBackendErrors(t *testing.T) {
	store := newMemoryKeyringStore()
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey("admin", "acme"), "not json"))

	_, err := store.Retrieve("admin", "acme")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrCredentialsNotFound))
}

// TestFilterRealmKeys_SeparatesRealms verifies List-style filtering returns only the
// requested realm's aliases from both layouts, never another realm's.
func TestFilterRealmKeys_SeparatesRealms(t *testing.T) {
	keys := []string{
		buildKeyringKey("prod_admin", "acme"),
		buildKeyringKey("admin", "acme_prod"),
		legacyRealmKeyringKey("reader", "acme"),
		legacyRealmKeyringKey("prod_reader", "acme"),
		buildKeyringKey("reader", "acme"),
		buildKeyringKey("solo", ""),
	}
	assert.Equal(t, []string{"prod_admin", "reader"}, (&keyringCredentialStore{}).filterRealmKeys(keys, "acme"))
	assert.Equal(t, []string{"admin"}, (&keyringCredentialStore{}).filterRealmKeys(keys, "acme_prod"))
}

// TestKeyringStore_DeleteReportsLegacyDeleteFailure verifies a backend failure while
// removing the previous-layout entry is reported, not silently ignored, since the entry
// would otherwise resurface through the read fallback.
func TestKeyringStore_DeleteReportsLegacyDeleteFailure(t *testing.T) {
	kr := keyring.NewMockKeyring(gomock.NewController(t))
	store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
	kr.EXPECT().Get(buildKeyringKey("admin", "acme")).Return("", keyring.ErrNotFound)
	kr.EXPECT().Delete(buildKeyringKey("admin", "acme")).Return(nil)
	kr.EXPECT().Delete(buildConfigDigestKey("admin", "acme")).Return(nil)
	kr.EXPECT().Get(legacyRealmKeyringKey("admin", "acme")).Return("", keyring.ErrNotFound)
	kr.EXPECT().Delete(legacyRealmKeyringKey("admin", "acme")).Return(assert.AnError)

	err := store.Delete("admin", "acme")
	require.ErrorIs(t, err, assert.AnError)
	require.ErrorIs(t, err, ErrCredentialStore)
}

// TestKeyringStore_DeleteStopsOnOwnershipReadFailure verifies a read failure other than
// not-found stops Delete before removing an entry whose owner was never read.
func TestKeyringStore_DeleteStopsOnOwnershipReadFailure(t *testing.T) {
	t.Run("current key", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Get(buildKeyringKey("admin", "acme")).Return("", assert.AnError)

		err := store.Delete("admin", "acme")
		require.ErrorIs(t, err, assert.AnError)
		require.ErrorIs(t, err, ErrCredentialStore)
	})

	t.Run("legacy key", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Get(buildKeyringKey("admin", "acme")).Return("", keyring.ErrNotFound)
		kr.EXPECT().Delete(buildKeyringKey("admin", "acme")).Return(nil)
		kr.EXPECT().Delete(buildConfigDigestKey("admin", "acme")).Return(nil)
		kr.EXPECT().Get(legacyRealmKeyringKey("admin", "acme")).Return("", assert.AnError)

		err := store.Delete("admin", "acme")
		require.ErrorIs(t, err, assert.AnError)
		require.ErrorIs(t, err, ErrCredentialStore)
	})
}

// TestEmptyRealmKeyIsUnambiguous pins which pre-realm keys realm-scoped callers may
// delete as their own legacy entry.
func TestEmptyRealmKeyIsUnambiguous(t *testing.T) {
	tests := []struct {
		alias string
		want  bool
	}{
		{alias: "deployer", want: true},
		{alias: "core-root/terraform", want: true},
		{alias: "prod_admin", want: false},  // Previous layout of realm "prod" + "admin".
		{alias: "prod__admin", want: false}, // Current layout of realm "prod" + "admin".
		{alias: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			assert.Equal(t, tt.want, EmptyRealmKeyIsUnambiguous(tt.alias))
		})
	}
}

func writeLegacyRealmEntry(t *testing.T, store *keyringCredentialStore, alias, realm, token string) {
	t.Helper()
	require.NoError(t, store.Store(alias, &types.OIDCCredentials{Token: token}, realm))
	raw, err := store.kr.Get(buildKeyringKey(alias, realm))
	require.NoError(t, err)
	require.NoError(t, store.kr.Delete(buildKeyringKey(alias, realm)))
	// Older versions wrote no alias into the envelope.
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey(alias, realm), stripEnvelopeAlias(t, raw)))
}

// Compile-time sentinel for the envelope field these tests rely on.
var _ = credentialEnvelope{Alias: ""}

// TestCredentialEnvelope_AliasBindsEntryToOwner covers a v1.234.1 alias such as "_admin": its
// previous-layout key "atmos_acme__admin" is also the current key of alias "admin". An entry that
// records the alias it was stored for is never returned for, migrated to, or deleted by another
// alias, while entries without an alias (older versions) keep working.
func TestCredentialEnvelope_AliasBindsEntryToOwner(t *testing.T) {
	sharedKey := buildKeyringKey("admin", "acme")
	require.Equal(t, "atmos_acme__admin", sharedKey)
	require.Equal(t, sharedKey, legacyRealmKeyringKey("_admin", "acme"))

	t.Run("round trip records the alias", func(t *testing.T) {
		store := newMemoryKeyringStore()
		require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "own"}, "acme"))
		raw, err := store.kr.Get(sharedKey)
		require.NoError(t, err)
		var env credentialEnvelope
		require.NoError(t, json.Unmarshal([]byte(raw), &env))
		assert.Equal(t, "admin", env.Alias)
		got, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
		assert.Equal(t, "own", got.(*types.OIDCCredentials).Token)
	})

	t.Run("an entry stored for another alias is not found, kept, and not deleted", func(t *testing.T) {
		store := newMemoryKeyringStore()
		other := &credentialEnvelope{Type: "oidc", Data: json.RawMessage(`{"token":"underscore-admin"}`), Alias: "_admin"}
		data, err := json.Marshal(other)
		require.NoError(t, err)
		require.NoError(t, store.kr.Set(sharedKey, string(data)))

		_, err = store.Retrieve("admin", "acme")
		require.ErrorIs(t, err, ErrCredentialsNotFound)

		require.NoError(t, store.Delete("admin", "acme"))
		kept, err := store.kr.Get(sharedKey)
		require.NoError(t, err, "another alias's entry must survive a delete of this alias")
		assert.Equal(t, string(data), kept, "and stay unchanged")
	})

	t.Run("an entry without an alias is still readable", func(t *testing.T) {
		store := newMemoryKeyringStore()
		require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "older"}, "acme"))
		raw, err := store.kr.Get(sharedKey)
		require.NoError(t, err)
		require.NoError(t, store.kr.Set(sharedKey, stripEnvelopeAlias(t, raw)))

		got, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
		assert.Equal(t, "older", got.(*types.OIDCCredentials).Token)
		require.NoError(t, store.Delete("admin", "acme"))
		_, err = store.kr.Get(sharedKey)
		require.ErrorIs(t, err, keyring.ErrNotFound)
	})
}
