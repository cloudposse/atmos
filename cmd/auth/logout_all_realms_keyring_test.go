package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

// allRealmsKeyringEntry is a decodable credential envelope as older versions stored it.
func allRealmsKeyringEntry(key string) string {
	return `{"type":"aws","data":{"access_key_id":"AKIA` + key + `","secret_access_key":"s"}}`
}

// TestDeleteRealmKeyringEntries_RemovesLegacyAndDigest verifies that
// `atmos auth logout --all-realms --keychain` removes, for each configured identity, the same
// entries the per-identity keychain logout removes: the current-layout entry, the ambiguous
// previous-layout key that older versions deleted as the identity's own, and the identity's
// configuration digest record. Entries of identities that are not configured are kept.
func TestDeleteRealmKeyringEntries_RemovesLegacyAndDigest(t *testing.T) {
	credtest.IsolateKeyring(t)
	store := credentials.NewCredentialStoreWithConfig(nil)
	require.Equal(t, types.CredentialStoreTypeFile, store.Type(), "tests must never reach the OS keyring")
	raw, err := credentials.OpenKeyring(nil)
	require.NoError(t, err)

	const realm = "acme"
	seeded := map[string]string{
		"atmos_acme__prod_admin": allRealmsKeyringEntry("current"),
		"atmos_acme_prod_admin":  allRealmsKeyringEntry("legacy"),
		"atmos_acme__other":      allRealmsKeyringEntry("other"),
	}
	for key, value := range seeded {
		require.NoError(t, raw.Set(key, value))
	}
	// The file keyring keeps configuration digest records outside the keyring.
	digests, ok := store.(credentials.ConfigDigestStore)
	require.True(t, ok)
	require.NoError(t, digests.StoreExternalConfigDigest("prod_admin", realm, credentials.ExternalConfigDigest{Digest: "d"}))
	require.NoError(t, digests.StoreExternalConfigDigest("other", realm, credentials.ExternalConfigDigest{Digest: "o"}))

	deleteRealmKeyringEntries(store, []string{"prod_admin"}, realm)

	for _, key := range []string{"atmos_acme__prod_admin", "atmos_acme_prod_admin"} {
		_, err := raw.Get(key)
		assert.ErrorIs(t, err, keyring.ErrNotFound, "keyring key %q must be deleted", key)
	}
	_, err = digests.RetrieveExternalConfigDigest("prod_admin", realm)
	require.ErrorIs(t, err, credentials.ErrDataNotFound, "the digest record must be deleted")
	_, err = digests.RetrieveExternalConfigDigest("other", realm)
	require.NoError(t, err, "an identity that is not being logged out keeps its digest record")
	got, err := raw.Get("atmos_acme__other")
	require.NoError(t, err, "an identity that is not being logged out keeps its entry")
	assert.Equal(t, seeded["atmos_acme__other"], got)
}

// TestDeleteRealmKeyringEntries_MissingEntriesAreNotErrors verifies the best-effort contract:
// nothing to delete is not a failure and does not panic, including for an empty realm.
func TestDeleteRealmKeyringEntries_MissingEntriesAreNotErrors(t *testing.T) {
	credtest.IsolateKeyring(t)
	store := credentials.NewCredentialStoreWithConfig(nil)

	assert.NotPanics(t, func() {
		deleteRealmKeyringEntries(store, []string{"prod_admin", "dev"}, "acme")
		deleteRealmKeyringEntries(store, []string{"prod_admin"}, "")
		deleteRealmKeyringEntries(store, nil, "acme")
	})
}
