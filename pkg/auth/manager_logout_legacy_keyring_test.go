package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema fields these tests rely on.
var _ = schema.Identity{Kind: "aws/user"}

// logoutKeyringFixture is a manager over an isolated file keyring with aws/user identities in the
// explicit realm "acme", plus the raw keyring so tests can seed keys as older versions wrote them.
type logoutKeyringFixture struct {
	m     *manager
	store types.CredentialStore
	raw   keyring.Keyring
}

func newLogoutKeyringFixture(t *testing.T, identityNames ...string) *logoutKeyringFixture {
	t.Helper()
	credtest.IsolateKeyring(t)
	store := credentials.NewCredentialStoreWithConfig(nil)
	require.Equal(t, types.CredentialStoreTypeFile, store.Type(), "tests must never reach the OS keyring")
	raw, err := credentials.OpenKeyring(nil)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	config := &schema.AuthConfig{Identities: map[string]schema.Identity{}}
	identities := map[string]types.Identity{}
	for _, name := range identityNames {
		config.Identities[name] = schema.Identity{Kind: "aws/user"}
		identity := types.NewMockIdentity(ctrl)
		identity.EXPECT().Logout(gomock.Any()).Return(nil).AnyTimes()
		identities[name] = identity
	}
	m := &manager{
		config:          config,
		credentialStore: store,
		providers:       map[string]types.Provider{},
		identities:      identities,
		realm:           realm.RealmInfo{Value: "acme", Source: realm.SourceConfig},
	}
	return &logoutKeyringFixture{m: m, store: store, raw: raw}
}

// seed writes a raw keyring entry and returns its value.
func (f *logoutKeyringFixture) seed(t *testing.T, key string) string {
	t.Helper()
	value := `{"type":"aws","data":{"access_key_id":"AKIA` + key + `","secret_access_key":"s"}}`
	require.NoError(t, f.raw.Set(key, value))
	return value
}

func (f *logoutKeyringFixture) assertGone(t *testing.T, key string) {
	t.Helper()
	_, err := f.raw.Get(key)
	require.ErrorIs(t, err, keyring.ErrNotFound, "keyring key %q must be deleted", key)
}

func (f *logoutKeyringFixture) assertKept(t *testing.T, key, value string) {
	t.Helper()
	got, err := f.raw.Get(key)
	require.NoError(t, err, "keyring key %q must be kept", key)
	assert.Equal(t, value, got)
}

// TestManager_LogoutKeychain_DeletesAmbiguousLegacyKey reproduces regression A3 of the
// 1.235.0 regression test: `atmos auth logout --keychain` reported the keys deleted but kept the
// identity's ambiguous previous-layout key "atmos_<realm>_<identity>", so a later interactive
// login offered to revive the deleted keys. Version 1.234.1 deleted that key as the identity's own.
// Current-layout keys of other realms and identities must be left alone.
func TestManager_LogoutKeychain_DeletesAmbiguousLegacyKey(t *testing.T) {
	f := newLogoutKeyringFixture(t, "prod_admin")
	f.seed(t, "atmos_acme__prod_admin")
	f.seed(t, "atmos_acme_prod_admin")
	otherRealmCurrent := f.seed(t, "atmos_acme_prod__admin")
	otherRealmSameAlias := f.seed(t, "atmos_other__prod_admin")
	otherRealmLegacy := f.seed(t, "atmos_other_prod_admin")

	require.NoError(t, f.m.Logout(context.Background(), "prod_admin", true))

	f.assertGone(t, "atmos_acme__prod_admin")
	f.assertGone(t, "atmos_acme_prod_admin")
	f.assertKept(t, "atmos_acme_prod__admin", otherRealmCurrent)
	f.assertKept(t, "atmos_other__prod_admin", otherRealmSameAlias)
	f.assertKept(t, "atmos_other_prod_admin", otherRealmLegacy)

	// Nothing is left for a later login to offer.
	legacyStore, ok := f.store.(credentials.AmbiguousLegacyRealmStore)
	require.True(t, ok)
	entry, err := legacyStore.FindAmbiguousLegacyRealmEntry("prod_admin", "acme")
	require.NoError(t, err)
	assert.Nil(t, entry)
}

// TestManager_LogoutAllKeychain_DeletesAmbiguousLegacyKeys covers `--all --keychain`: every
// aws/user identity's ambiguous previous-layout key is deleted.
func TestManager_LogoutAllKeychain_DeletesAmbiguousLegacyKeys(t *testing.T) {
	f := newLogoutKeyringFixture(t, "prod_admin", "ci_user", "reader")
	f.seed(t, "atmos_acme_prod_admin")
	f.seed(t, "atmos_acme_ci_user")
	f.seed(t, "atmos_acme_reader")
	otherRealmCurrent := f.seed(t, "atmos_acme_ci__user")

	require.NoError(t, f.m.LogoutAll(context.Background(), true))

	f.assertGone(t, "atmos_acme_prod_admin")
	f.assertGone(t, "atmos_acme_ci_user")
	f.assertGone(t, "atmos_acme_reader")
	f.assertKept(t, "atmos_acme_ci__user", otherRealmCurrent)
}

// TestManager_LogoutWithoutKeychain_KeepsLegacyKeys is the negative path: without --keychain the
// keyring is preserved, legacy keys included.
func TestManager_LogoutWithoutKeychain_KeepsLegacyKeys(t *testing.T) {
	f := newLogoutKeyringFixture(t, "prod_admin")
	current := f.seed(t, "atmos_acme__prod_admin")
	legacy := f.seed(t, "atmos_acme_prod_admin")

	require.NoError(t, f.m.Logout(context.Background(), "prod_admin", false))
	require.NoError(t, f.m.LogoutAll(context.Background(), false))

	f.assertKept(t, "atmos_acme__prod_admin", current)
	f.assertKept(t, "atmos_acme_prod_admin", legacy)
}

// failingLegacyDeleterStore is the real credential store with a failing ambiguous legacy delete.
type failingLegacyDeleterStore struct {
	types.CredentialStore
}

func (failingLegacyDeleterStore) DeleteAmbiguousLegacyRealmEntry(string, string) (*credentials.AmbiguousLegacyRealmEntry, error) {
	return nil, assert.AnError
}

// TestManager_LogoutKeychain_AmbiguousLegacyDeleteFailure verifies a failed delete of the legacy
// key is reported as a partial logout, never as a completed one, for an explicit --keychain
// logout, and is only logged for a forced best-effort delete.
func TestManager_LogoutKeychain_AmbiguousLegacyDeleteFailure(t *testing.T) {
	f := newLogoutKeyringFixture(t, "prod_admin")
	f.m.credentialStore = failingLegacyDeleterStore{CredentialStore: f.store}

	err := f.m.Logout(context.Background(), "prod_admin", true)
	require.ErrorIs(t, err, errUtils.ErrPartialLogout)
	require.ErrorIs(t, err, errUtils.ErrKeyringDeletion)
	require.ErrorIs(t, err, assert.AnError)

	assert.Empty(t, f.m.deleteAmbiguousLegacyKeyringEntry("prod_admin", logKeyIdentity, true))
}

// TestManager_LogoutKeychain_AmbiguousLegacyDeleteSkipped covers the cases with nothing to do: a
// store that cannot delete such entries, and no realm (the previous layout had no realm part).
func TestManager_LogoutKeychain_AmbiguousLegacyDeleteSkipped(t *testing.T) {
	f := newLogoutKeyringFixture(t, "prod_admin")
	f.m.credentialStore = types.NewMockCredentialStore(gomock.NewController(t))
	assert.Empty(t, f.m.deleteAmbiguousLegacyKeyringEntry("prod_admin", logKeyIdentity, false))

	f.m.credentialStore = failingLegacyDeleterStore{CredentialStore: f.store}
	f.m.realm = realm.RealmInfo{}
	assert.Empty(t, f.m.deleteAmbiguousLegacyKeyringEntry("prod_admin", logKeyIdentity, false))
}

// TestManager_LogoutProviderKeychain_DeletesAmbiguousLegacyKey verifies provider keychain logout
// also deletes the provider's ambiguous previous-layout key, as v1.234.1 did, and leaves the
// current-layout key of the other realm and name split alone.
func TestManager_LogoutProviderKeychain_DeletesAmbiguousLegacyKey(t *testing.T) {
	f := newLogoutKeyringFixture(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	provider := types.NewMockProvider(gomock.NewController(t))
	provider.EXPECT().Logout(gomock.Any()).Return(nil)
	f.m.providers = map[string]types.Provider{"corp_sso": provider}
	f.seed(t, "atmos_acme_corp_sso")
	otherRealmCurrent := f.seed(t, "atmos_acme_corp__sso")

	require.NoError(t, f.m.LogoutProvider(context.Background(), "corp_sso", true))

	f.assertGone(t, "atmos_acme_corp_sso")
	f.assertKept(t, "atmos_acme_corp__sso", otherRealmCurrent)
}

// TestDescribeLegacyRealmOwners verifies the other owners named in the logout warning.
func TestDescribeLegacyRealmOwners(t *testing.T) {
	assert.Equal(t, "admin in realm acme_prod or b in realm acme_prod_a", describeLegacyRealmOwners([]credentials.LegacyRealmOwner{
		{Realm: "acme_prod", Alias: "admin"}, {Realm: "acme_prod_a", Alias: "b"},
	}))
	assert.Empty(t, describeLegacyRealmOwners(nil))
}
