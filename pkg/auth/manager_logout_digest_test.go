package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// seedDigest records a configuration digest for name in the fixture's realm, as a login does for
// credentials an identity keeps in its own files.
func (f *logoutKeyringFixture) seedDigest(t *testing.T, name string) credentials.ConfigDigestStore {
	t.Helper()
	store, ok := f.store.(credentials.ConfigDigestStore)
	require.True(t, ok)
	require.NoError(t, store.StoreExternalConfigDigest(name, f.m.realm.Value, credentials.ExternalConfigDigest{Digest: "d-" + name}))
	return store
}

func assertDigestGone(t *testing.T, store credentials.ConfigDigestStore, name, realmValue string) {
	t.Helper()
	_, err := store.RetrieveExternalConfigDigest(name, realmValue)
	require.ErrorIs(t, err, credentials.ErrDataNotFound, "the digest record of %q must be deleted", name)
}

// TestManager_Logout_DeletesConfigDigestRecord reproduces regression A7 of the 1.235.0 regression
// test: every logout left the identity's "atmos-config-digest_<realm>__<identity>" keyring
// entry behind. Logout removes the credential files that record describes, so it must remove the
// record too, with and without --keychain, and leave other identities' records alone.
func TestManager_Logout_DeletesConfigDigestRecord(t *testing.T) {
	for _, deleteKeychain := range []bool{false, true} {
		t.Run(map[bool]string{false: "without keychain", true: "with keychain"}[deleteKeychain], func(t *testing.T) {
			f := newLogoutKeyringFixture(t, "dev-user", "other-user")
			store := f.seedDigest(t, "dev-user")
			f.seedDigest(t, "other-user")

			require.NoError(t, f.m.Logout(context.Background(), "dev-user", deleteKeychain))

			assertDigestGone(t, store, "dev-user", "acme")
			record, err := store.RetrieveExternalConfigDigest("other-user", "acme")
			require.NoError(t, err, "another identity's digest record must be kept")
			assert.Equal(t, "d-other-user", record.Digest)
		})
	}
}

// failingDigestDeleteStore is the real credential store with a failing digest record delete.
type failingDigestDeleteStore struct {
	credentials.ConfigDigestStore
	types.CredentialStore
	deleteCalls *int
}

func (s failingDigestDeleteStore) DeleteExternalConfigDigest(string, string) error {
	*s.deleteCalls++
	return assert.AnError
}

// TestManager_Logout_DigestDeleteFailureIsNonFatal verifies a failed delete of the non-secret
// digest record never fails the logout, like the other best-effort digest bookkeeping.
func TestManager_Logout_DigestDeleteFailureIsNonFatal(t *testing.T) {
	f := newLogoutKeyringFixture(t, "dev-user")
	digests := f.seedDigest(t, "dev-user")
	deleteCalls := 0
	f.m.credentialStore = failingDigestDeleteStore{ConfigDigestStore: digests, CredentialStore: f.store, deleteCalls: &deleteCalls}

	require.NoError(t, f.m.Logout(context.Background(), "dev-user", false))
	require.NotZero(t, deleteCalls, "logout must attempt the digest record delete")
}

// TestManager_LogoutAll_DeletesConfigDigestRecords covers `atmos auth logout --all`.
func TestManager_LogoutAll_DeletesConfigDigestRecords(t *testing.T) {
	for _, deleteKeychain := range []bool{false, true} {
		t.Run(map[bool]string{false: "without keychain", true: "with keychain"}[deleteKeychain], func(t *testing.T) {
			f := newLogoutKeyringFixture(t, "dev-user", "ci_user")
			store := f.seedDigest(t, "dev-user")
			f.seedDigest(t, "ci_user")

			require.NoError(t, f.m.LogoutAll(context.Background(), deleteKeychain))

			assertDigestGone(t, store, "dev-user", "acme")
			assertDigestGone(t, store, "ci_user", "acme")
		})
	}
}
