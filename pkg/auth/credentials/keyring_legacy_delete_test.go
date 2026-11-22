package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

func TestDeleteAmbiguousLegacyRealmEntry_DeletesEntryAndNamesOtherOwners(t *testing.T) {
	store := newMemoryKeyringStore()
	seedAmbiguousLegacyKeys(t, store)
	// The current-layout key of the other split must survive.
	require.NoError(t, store.Store("admin", &types.AWSCredentials{AccessKeyID: "AKIAOTHER", SecretAccessKey: "s"}, "acme_prod"))
	otherRaw, err := store.kr.Get(buildKeyringKey("admin", "acme_prod"))
	require.NoError(t, err)

	entry, err := store.DeleteAmbiguousLegacyRealmEntry("prod_admin", "acme")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "atmos_acme_prod_admin", entry.Key)
	assert.Nil(t, entry.Credentials, "the deleted entry is never decoded")
	assert.Equal(t, []LegacyRealmOwner{{Realm: "acme_prod", Alias: "admin"}}, entry.OtherOwners)

	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
	got, err := store.kr.Get(buildKeyringKey("admin", "acme_prod"))
	require.NoError(t, err)
	assert.Equal(t, otherRaw, got)

	// A second call finds nothing, so nothing is reported as deleted.
	entry, err = store.DeleteAmbiguousLegacyRealmEntry("prod_admin", "acme")
	require.NoError(t, err)
	assert.Nil(t, entry)
}

func TestDeleteAmbiguousLegacyRealmEntry_DeletesUndecodableEntry(t *testing.T) {
	store := newMemoryKeyringStore()
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey("prod_admin", "acme"), "{not json"))

	entry, err := store.DeleteAmbiguousLegacyRealmEntry("prod_admin", "acme")
	require.NoError(t, err)
	require.NotNil(t, entry)
	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestDeleteAmbiguousLegacyRealmEntry_NothingToDelete(t *testing.T) {
	tests := []struct {
		name, alias, realm string
	}{
		{name: "no realm", alias: "prod_admin", realm: ""},
		{name: "unambiguous key (Delete removes it)", alias: "admin", realm: "acme"},
		{name: "auto realm (Delete removes it)", alias: "prod_admin", realm: autoTestRealm},
		{name: "key that is also a current-layout key", alias: "x__y", realm: "acme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryKeyringStore()
			key := legacyRealmKeyringKey(tt.alias, tt.realm)
			require.NoError(t, store.kr.Set(key, "kept"))

			entry, err := store.DeleteAmbiguousLegacyRealmEntry(tt.alias, tt.realm)
			require.NoError(t, err)
			assert.Nil(t, entry)
			got, err := store.kr.Get(key)
			require.NoError(t, err)
			assert.Equal(t, "kept", got)
		})
	}
}

func TestDeleteAmbiguousLegacyRealmEntry_KeyringFailures(t *testing.T) {
	legacy := legacyRealmKeyringKey("prod_admin", "acme")

	t.Run("probe fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Has(legacy).Return(false, assert.AnError)

		entry, err := store.DeleteAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		require.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, entry)
	})

	t.Run("delete fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Has(legacy).Return(true, nil)
		kr.EXPECT().Delete(legacy).Return(assert.AnError)

		entry, err := store.DeleteAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		require.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, entry, "a failed delete is never reported as a deletion")
	})
}
