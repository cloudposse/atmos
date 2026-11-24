package credentials

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

// TestKeyringStore_MigratesUnambiguousLegacyRealmKey reproduces the #3720 item 3.4
// follow-up: an entry found through the previous-layout fallback is copied under the
// current "atmos_<realm>__<alias>" key, so later reads no longer depend on the fallback. The
// legacy key is kept, so a downgrade to v1.234.1 (which reads only the legacy key) still finds it.
func TestKeyringStore_MigratesUnambiguousLegacyRealmKey(t *testing.T) {
	store := newMemoryKeyringStore()
	writeLegacyRealmEntry(t, store, "admin", "acme", "legacy-token")
	legacyRaw, err := store.kr.Get(legacyRealmKeyringKey("admin", "acme"))
	require.NoError(t, err)

	got, err := store.Retrieve("admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "legacy-token", got.(*types.OIDCCredentials).Token)

	migrated, err := store.kr.Get(buildKeyringKey("admin", "acme"))
	require.NoError(t, err, "the legacy entry must be rewritten under the current key")
	assert.Equal(t, legacyRaw, migrated, "the entry must be copied unchanged")
	kept, err := store.kr.Get(legacyRealmKeyringKey("admin", "acme"))
	require.NoError(t, err, "the legacy key must be kept for a downgrade")
	assert.Equal(t, legacyRaw, kept)

	// A later read is served by the current key alone.
	got, err = store.Retrieve("admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "legacy-token", got.(*types.OIDCCredentials).Token)
}

// TestKeyringStore_MigratesLegacyAWSUserLongLivedKeys covers the aws/user identity, which
// reads the long-lived keys stored by `atmos auth user configure` through the same Retrieve
// call: the keys survive the layout change and are copied to the current key. This reproduces
// the 1.235.0 downgrade regression: after one login with the new version, v1.234.1 must still
// find the long-lived keys under the legacy key.
func TestKeyringStore_MigratesLegacyAWSUserLongLivedKeys(t *testing.T) {
	store := newMemoryKeyringStore()
	longLived := &types.AWSCredentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", MfaArn: "arn:aws:iam::111111111111:mfa/me"}
	require.NoError(t, store.Store("deployer", longLived, "acme"))
	raw, err := store.kr.Get(buildKeyringKey("deployer", "acme"))
	require.NoError(t, err)
	require.NoError(t, store.kr.Delete(buildKeyringKey("deployer", "acme")))
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey("deployer", "acme"), raw))

	got, err := store.Retrieve("deployer", "acme")
	require.NoError(t, err)
	assert.Equal(t, longLived, got)

	legacyRaw, err := store.kr.Get(legacyRealmKeyringKey("deployer", "acme"))
	require.NoError(t, err, "v1.234.1 reads only the legacy key, so it must survive the migration")
	assert.Equal(t, raw, legacyRaw)
	got, err = store.Retrieve("deployer", "acme")
	require.NoError(t, err)
	assert.Equal(t, longLived, got)

	// Removing the credentials removes both copies, so nothing is left for either version.
	require.NoError(t, store.Delete("deployer", "acme"))
	_, err = store.kr.Get(buildKeyringKey("deployer", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
	_, err = store.kr.Get(legacyRealmKeyringKey("deployer", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
	_, err = store.Retrieve("deployer", "acme")
	require.ErrorIs(t, err, ErrCredentialsNotFound)
}

// TestKeyringStore_MigratedEntryReadsOnlyTheCurrentKey verifies that once copied, the current
// key is the only one read: newer credentials stored under it win over the kept legacy copy.
func TestKeyringStore_MigratedEntryReadsOnlyTheCurrentKey(t *testing.T) {
	store := newMemoryKeyringStore()
	writeLegacyRealmEntry(t, store, "admin", "acme", "legacy-token")
	_, err := store.Retrieve("admin", "acme")
	require.NoError(t, err)

	require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "fresh-token"}, "acme"))
	got, err := store.Retrieve("admin", "acme")
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", got.(*types.OIDCCredentials).Token)
}

// TestKeyringStore_AmbiguousLegacyKeyIsNeverMigrated verifies the deliberate safety choice
// is unchanged: an old-layout key that another realm and alias split could have written is
// neither read, rewritten, nor deleted.
func TestKeyringStore_AmbiguousLegacyKeyIsNeverMigrated(t *testing.T) {
	store := newMemoryKeyringStore()
	writeLegacyRealmEntry(t, store, "prod_admin", "acme", "maybe-another-realm")

	_, err := store.Retrieve("prod_admin", "acme")
	require.ErrorIs(t, err, ErrCredentialsNotFound)

	_, err = store.kr.Get(buildKeyringKey("prod_admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound, "an ambiguous legacy entry must not be copied")
	_, err = store.kr.Get(legacyRealmKeyringKey("prod_admin", "acme"))
	require.NoError(t, err, "an ambiguous legacy entry must be kept")
}

// TestKeyringStore_CorruptLegacyEntryIsNotMigrated verifies only an entry that decodes is
// copied; a corrupt legacy entry is reported and left where it is.
func TestKeyringStore_CorruptLegacyEntryIsNotMigrated(t *testing.T) {
	store := newMemoryKeyringStore()
	require.NoError(t, store.kr.Set(legacyRealmKeyringKey("admin", "acme"), `{"type":"unknown-tag","data":{}}`))

	_, err := store.Retrieve("admin", "acme")
	require.ErrorIs(t, err, ErrUnknownCredentialType)

	_, err = store.kr.Get(buildKeyringKey("admin", "acme"))
	require.ErrorIs(t, err, keyring.ErrNotFound)
	_, err = store.kr.Get(legacyRealmKeyringKey("admin", "acme"))
	require.NoError(t, err)
}

// TestKeyringStore_LegacyMigrationIsBestEffort verifies a failed rewrite never fails the
// read and keeps the legacy key, so the credential stays reachable through the fallback.
func TestKeyringStore_LegacyMigrationIsBestEffort(t *testing.T) {
	raw := legacyEnvelope(t, "legacy-token")
	current, legacy := buildKeyringKey("admin", "acme"), legacyRealmKeyringKey("admin", "acme")

	t.Run("rewrite fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(current).Return("", keyring.ErrNotFound),
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, nil),
			kr.EXPECT().Set(current, raw).Return(assert.AnError),
		)
		// No Delete is expected: the legacy key must be kept when the rewrite fails.

		got, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
		assert.Equal(t, "legacy-token", got.(*types.OIDCCredentials).Token)
	})

	t.Run("legacy key is never deleted", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(current).Return("", keyring.ErrNotFound),
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, nil),
			kr.EXPECT().Set(current, raw).Return(nil),
		)
		// No Delete is expected: the legacy copy is kept for a downgrade.

		got, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
		assert.Equal(t, "legacy-token", got.(*types.OIDCCredentials).Token)
	})

	t.Run("current key written concurrently", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		// Another process stored fresh credentials between the two reads: the legacy entry
		// must not overwrite them. It stays for the next Delete to remove.
		gomock.InOrder(
			kr.EXPECT().Get(current).Return("", keyring.ErrNotFound),
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(true, nil),
		)

		_, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
	})

	t.Run("probe fails", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
		gomock.InOrder(
			kr.EXPECT().Get(current).Return("", keyring.ErrNotFound),
			kr.EXPECT().Get(legacy).Return(raw, nil),
			kr.EXPECT().Has(current).Return(false, assert.AnError),
		)

		_, err := store.Retrieve("admin", "acme")
		require.NoError(t, err)
	})
}

// legacyEnvelope returns a stored OIDC envelope as the previous layout persisted it.
func legacyEnvelope(t *testing.T, token string) string {
	t.Helper()
	store := newMemoryKeyringStore()
	require.NoError(t, store.Store("x", &types.OIDCCredentials{Token: token}, ""))
	raw, err := store.kr.Get(buildKeyringKey("x", ""))
	require.NoError(t, err)
	return stripEnvelopeAlias(t, raw)
}

// stripEnvelopeAlias removes the alias an envelope carries, giving the alias-less shape written by
// versions before the alias was recorded.
func stripEnvelopeAlias(t *testing.T, raw string) string {
	t.Helper()
	var env credentialEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	env.Alias = ""
	stripped, err := json.Marshal(&env)
	require.NoError(t, err)
	return string(stripped)
}
