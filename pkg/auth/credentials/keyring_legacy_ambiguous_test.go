package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

// seedAmbiguousLegacyKeys stores long-lived AWS keys under the previous-layout key of realm
// "acme" + alias "prod_admin", which realm "acme_prod" + alias "admin" shared.
func seedAmbiguousLegacyKeys(t *testing.T, store *keyringCredentialStore) (*types.AWSCredentials, string) {
	t.Helper()
	keys := &types.AWSCredentials{AccessKeyID: "AKIALEGACY", SecretAccessKey: "legacy-secret"}
	require.NoError(t, store.Store("prod_admin", keys, "acme"))
	raw, err := store.kr.Get(buildKeyringKey("prod_admin", "acme"))
	require.NoError(t, err)
	require.NoError(t, store.kr.Delete(buildKeyringKey("prod_admin", "acme")))
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey("prod_admin", "acme"), raw))
	return keys, raw
}

func TestFindAmbiguousLegacyRealmEntry_ReportsEntryAndOtherOwners(t *testing.T) {
	store := newMemoryKeyringStore()
	keys, raw := seedAmbiguousLegacyKeys(t, store)

	entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "atmos_acme_prod_admin", entry.Key)
	assert.Equal(t, keys, entry.Credentials)
	assert.Equal(t, []LegacyRealmOwner{{Realm: "acme_prod", Alias: "admin"}}, entry.OtherOwners)

	// The other split sees the same entry and names this pair as the other owner.
	entry, err = store.FindAmbiguousLegacyRealmEntry("admin", "acme_prod")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, []LegacyRealmOwner{{Realm: "acme", Alias: "prod_admin"}}, entry.OtherOwners)

	// Looking is read-only.
	got, err := store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.NoError(t, err)
	assert.Equal(t, raw, got)
	_, err = store.kr.Get(buildKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestFindAmbiguousLegacyRealmEntry_NothingToOffer(t *testing.T) {
	t.Run("current key exists", func(t *testing.T) {
		store := newMemoryKeyringStore()
		seedAmbiguousLegacyKeys(t, store)
		require.NoError(t, store.Store("prod_admin", &types.AWSCredentials{AccessKeyID: "AKIANEW", SecretAccessKey: "s"}, "acme"))

		entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)
	})

	t.Run("no legacy key", func(t *testing.T) {
		entry, err := newMemoryKeyringStore().FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)
	})

	t.Run("corrupt legacy entry", func(t *testing.T) {
		store := newMemoryKeyringStore()
		require.NoError(t, store.kr.Set(legacyRealmKeyringKey("prod_admin", "acme"), "{not json"))
		entry, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)

		require.NoError(t, store.kr.Set(legacyRealmKeyringKey("prod_admin", "acme"), `{"type":"unknown-tag","data":{}}`))
		entry, err = store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)
	})

	// Unambiguous keys are migrated by Retrieve itself, so they are never offered.
	t.Run("unambiguous legacy key", func(t *testing.T) {
		store := newMemoryKeyringStore()
		writeLegacyRealmEntry(t, store, "admin", "acme", "token")
		entry, err := store.FindAmbiguousLegacyRealmEntry("admin", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)
	})

	t.Run("no realm", func(t *testing.T) {
		entry, err := newMemoryKeyringStore().FindAmbiguousLegacyRealmEntry("prod_admin", "")
		require.NoError(t, err)
		assert.Nil(t, entry)
	})

	// "atmos_acme_x__y" is also the current key of realm "acme_x" + alias "y": never offered.
	t.Run("legacy key is also a current-layout key", func(t *testing.T) {
		store := newMemoryKeyringStore()
		require.NoError(t, store.Store("y", &types.AWSCredentials{AccessKeyID: "AKIALIVE", SecretAccessKey: "s"}, "acme_x"))
		require.Equal(t, buildKeyringKey("y", "acme_x"), legacyRealmKeyringKey("x__y", "acme"))

		entry, err := store.FindAmbiguousLegacyRealmEntry("x__y", "acme")
		require.NoError(t, err)
		assert.Nil(t, entry)
		require.ErrorIs(t, store.AdoptAmbiguousLegacyRealmEntry("x__y", "acme"), ErrCredentialsNotFound)
		_, err = store.Retrieve("y", "acme_x")
		require.NoError(t, err, "the live entry of the other realm must be untouched")
	})
}

func TestFindAmbiguousLegacyRealmEntry_KeyringErrors(t *testing.T) {
	current, legacy := buildKeyringKey("prod_admin", "acme"), legacyRealmKeyringKey("prod_admin", "acme")

	t.Run("probe fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Has(current).Return(false, assert.AnError)

		_, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("read fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Has(current).Return(false, nil)
		kr.EXPECT().Get(legacy).Return("", assert.AnError)

		_, err := store.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestAdoptAmbiguousLegacyRealmEntry_MovesEntry(t *testing.T) {
	store := newMemoryKeyringStore()
	keys, raw := seedAmbiguousLegacyKeys(t, store)

	require.NoError(t, store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"))

	moved, err := store.kr.Get(buildKeyringKey("prod_admin", "acme"))
	require.NoError(t, err)
	assert.Equal(t, raw, moved, "the entry must be copied unchanged")
	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound, "the confirmed legacy entry must be deleted")
	got, err := store.Retrieve("prod_admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, keys, got)

	// The other split no longer finds an entry that is now known not to be its own.
	entry, err := store.FindAmbiguousLegacyRealmEntry("admin", "acme_prod")
	require.NoError(t, err)
	assert.Nil(t, entry)
}

func TestAdoptAmbiguousLegacyRealmEntry_NeverOverwritesCurrentKey(t *testing.T) {
	store := newMemoryKeyringStore()
	seedAmbiguousLegacyKeys(t, store)
	newer := &types.AWSCredentials{AccessKeyID: "AKIANEWER", SecretAccessKey: "newer"}
	require.NoError(t, store.Store("prod_admin", newer, "acme"))

	require.NoError(t, store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"))

	got, err := store.Retrieve("prod_admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, newer, got, "credentials stored meanwhile must win")
	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestAdoptAmbiguousLegacyRealmEntry_Failures(t *testing.T) {
	current, legacy := buildKeyringKey("prod_admin", "acme"), legacyRealmKeyringKey("prod_admin", "acme")
	_, raw := seedAmbiguousLegacyKeys(t, newMemoryKeyringStore())

	t.Run("unambiguous key is not adoptable", func(t *testing.T) {
		require.ErrorIs(t, newMemoryKeyringStore().AdoptAmbiguousLegacyRealmEntry("admin", "acme"), ErrCredentialsNotFound)
	})

	t.Run("entry vanished", func(t *testing.T) {
		require.ErrorIs(t, newMemoryKeyringStore().AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"), ErrCredentialsNotFound)
	})

	t.Run("read fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		kr.EXPECT().Get(legacy).Return("", assert.AnError)
		require.ErrorIs(t, store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"), assert.AnError)
	})

	t.Run("probe fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, assert.AnError),
		)
		require.ErrorIs(t, store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"), assert.AnError)
	})

	t.Run("write fails keeps the legacy key", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, nil),
			kr.EXPECT().Set(current, raw).Return(assert.AnError),
		)
		// No Delete is expected.
		err := store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("legacy delete failure is not fatal", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, nil),
			kr.EXPECT().Set(current, raw).Return(nil),
			kr.EXPECT().Delete(legacy).Return(assert.AnError),
		)
		require.NoError(t, store.AdoptAmbiguousLegacyRealmEntry("prod_admin", "acme"))
	})
}

func TestOtherLegacyRealmOwners(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		realm string
		want  []LegacyRealmOwner
	}{
		{name: "alias with underscore", alias: "prod_admin", realm: "acme", want: []LegacyRealmOwner{{Realm: "acme_prod", Alias: "admin"}}},
		{name: "realm with underscore", alias: "admin", realm: "acme_prod", want: []LegacyRealmOwner{{Realm: "acme", Alias: "prod_admin"}}},
		{name: "several splits", alias: "b_c", realm: "a_x", want: []LegacyRealmOwner{
			{Realm: "a", Alias: "x_b_c"},
			{Realm: "a_x_b", Alias: "c"},
		}},
		// A split whose realm ends with "-" is not a valid realm, and an empty alias is skipped.
		{name: "invalid realms and empty aliases are skipped", alias: "x_", realm: "a-", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, otherLegacyRealmOwners(tt.alias, tt.realm))
		})
	}
}
