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

// TestCredentialEnvelope_DigestRoundTrip verifies the digest is stored with the credentials and
// read back, and that entries stored without one report "".
func TestCredentialEnvelope_DigestRoundTrip(t *testing.T) {
	store := newMemoryKeyringStore()
	require.NoError(t, store.StoreWithConfigDigest("mid", &types.OIDCCredentials{Token: "t"}, "acme", "abc123"))

	creds, digest, err := store.RetrieveWithConfigDigest("mid", "acme")
	require.NoError(t, err)
	assert.Equal(t, "abc123", digest)
	assert.Equal(t, "t", creds.(*types.OIDCCredentials).Token)

	// Retrieve keeps returning the credentials alone.
	plain, err := store.Retrieve("mid", "acme")
	require.NoError(t, err)
	assert.Equal(t, creds, plain)

	require.NoError(t, store.Store("mid", &types.OIDCCredentials{Token: "t2"}, "acme"))
	_, digest, err = store.RetrieveWithConfigDigest("mid", "acme")
	require.NoError(t, err)
	assert.Empty(t, digest, "Store must not keep a digest from an earlier write")

	_, _, err = store.RetrieveWithConfigDigest("missing", "acme")
	require.ErrorIs(t, err, ErrCredentialsNotFound)
}

// TestCredentialEnvelope_CompatibleWithOlderVersions pins the on-disk compatibility contract:
// an envelope without a digest is byte-identical to the previous format, and an older version
// (which decodes into a struct without the field) reads an envelope with a digest unchanged.
func TestCredentialEnvelope_CompatibleWithOlderVersions(t *testing.T) {
	type olderEnvelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	store := newMemoryKeyringStore()

	require.NoError(t, store.Store("a", &types.OIDCCredentials{Token: "t"}, "acme"))
	raw, err := store.kr.Get(buildKeyringKey("a", "acme"))
	require.NoError(t, err)
	assert.Equal(t, `{"type":"oidc","data":{"token":"t"}}`, raw)

	require.NoError(t, store.StoreWithConfigDigest("b", &types.OIDCCredentials{Token: "t"}, "acme", "abc123"))
	raw, err = store.kr.Get(buildKeyringKey("b", "acme"))
	require.NoError(t, err)
	assert.Equal(t, `{"type":"oidc","data":{"token":"t"},"config_digest":"abc123"}`, raw)

	var older olderEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &older))
	assert.Equal(t, "oidc", older.Type)
	decoded, err := unmarshalCredential(credentialEnvelope{Type: older.Type, Data: older.Data})
	require.NoError(t, err)
	assert.Equal(t, "t", decoded.(*types.OIDCCredentials).Token)
}

// TestExternalConfigDigest_RoundTripAndCleanup verifies the record of credentials kept outside
// the store is realm-scoped, never listed as an alias, and removed with the alias.
func TestExternalConfigDigest_RoundTripAndCleanup(t *testing.T) {
	store := newMemoryKeyringStore()
	record := ExternalConfigDigest{Digest: "abc123", Expiration: "2026-11-22T12:00:00Z"}
	require.NoError(t, store.StoreExternalConfigDigest("mid", "acme", record))

	got, err := store.RetrieveExternalConfigDigest("mid", "acme")
	require.NoError(t, err)
	assert.Equal(t, record, got)

	_, err = store.RetrieveExternalConfigDigest("mid", "other")
	require.ErrorIs(t, err, ErrDataNotFound, "records are realm-scoped")

	require.NoError(t, store.Store("mid", &types.OIDCCredentials{Token: "t"}, "acme"))
	aliases, err := store.List("acme")
	require.NoError(t, err)
	assert.Equal(t, []string{"mid"}, aliases, "digest records must never be listed as credential aliases")
	aliases, err = store.List("")
	require.NoError(t, err)
	assert.Equal(t, []string{"acme__mid"}, aliases)

	require.NoError(t, store.Delete("mid", "acme"))
	_, err = store.RetrieveExternalConfigDigest("mid", "acme")
	require.ErrorIs(t, err, ErrDataNotFound, "Delete must remove the digest record too")

	// DeleteExternalConfigDigest removes only the record, and is idempotent.
	require.NoError(t, store.Store("mid", &types.OIDCCredentials{Token: "t"}, "acme"))
	require.NoError(t, store.StoreExternalConfigDigest("mid", "acme", record))
	require.NoError(t, store.DeleteExternalConfigDigest("mid", "acme"))
	require.NoError(t, store.DeleteExternalConfigDigest("mid", "acme"))
	_, err = store.RetrieveExternalConfigDigest("mid", "acme")
	require.ErrorIs(t, err, ErrDataNotFound)
	_, err = store.Retrieve("mid", "acme")
	require.NoError(t, err, "the credentials are kept")
}

// TestExternalConfigDigest_Errors verifies backend and decoding failures are reported.
func TestExternalConfigDigest_Errors(t *testing.T) {
	t.Run("corrupt record", func(t *testing.T) {
		store := newMemoryKeyringStore()
		require.NoError(t, store.kr.Set(buildConfigDigestKey("mid", "acme"), "not json"))
		_, err := store.RetrieveExternalConfigDigest("mid", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		assert.NotErrorIs(t, err, ErrDataNotFound)
	})

	t.Run("backend failures", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeSystemKeyring}
		kr.EXPECT().Get(buildConfigDigestKey("mid", "acme")).Return("", assert.AnError)
		kr.EXPECT().Set(buildConfigDigestKey("mid", "acme"), gomock.Any()).Return(assert.AnError)

		_, err := store.RetrieveExternalConfigDigest("mid", "acme")
		require.ErrorIs(t, err, assert.AnError)
		assert.NotErrorIs(t, err, ErrDataNotFound)
		err = store.StoreExternalConfigDigest("mid", "acme", ExternalConfigDigest{Digest: "d"})
		require.ErrorIs(t, err, assert.AnError)
		require.ErrorIs(t, err, ErrCredentialStore)
	})

	t.Run("record delete failure does not fail Delete", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeSystemKeyring}
		kr.EXPECT().Delete(buildKeyringKey("mid", "acme")).Return(nil)
		kr.EXPECT().Delete(buildConfigDigestKey("mid", "acme")).Return(assert.AnError)
		kr.EXPECT().Delete(legacyRealmKeyringKey("mid", "acme")).Return(nil)
		require.NoError(t, store.Delete("mid", "acme"))
	})

	t.Run("record delete failure is reported by DeleteExternalConfigDigest", func(t *testing.T) {
		kr := keyring.NewMockKeyring(gomock.NewController(t))
		store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeSystemKeyring}
		kr.EXPECT().Delete(buildConfigDigestKey("mid", "acme")).Return(assert.AnError)
		err := store.DeleteExternalConfigDigest("mid", "acme")
		require.ErrorIs(t, err, assert.AnError)
		require.ErrorIs(t, err, ErrCredentialStore)
	})
}

// TestSupportsConfigDigests verifies only the noop backend, which persists nothing, opts out.
func TestSupportsConfigDigests(t *testing.T) {
	assert.True(t, newMemoryKeyringStore().SupportsConfigDigests())
	assert.True(t, (&keyringCredentialStore{storeType: types.CredentialStoreTypeFile}).SupportsConfigDigests())
	assert.True(t, (&keyringCredentialStore{storeType: types.CredentialStoreTypeSystemKeyring}).SupportsConfigDigests())
	assert.False(t, newNoopKeyringStore().SupportsConfigDigests())
}

// TestBuildConfigDigestKey pins the record key layout, which mirrors buildKeyringKey under a
// prefix that List never strips.
func TestBuildConfigDigestKey(t *testing.T) {
	assert.Equal(t, "atmos-config-digest_acme__core/admin", buildConfigDigestKey("core/admin", "acme"))
	assert.Equal(t, "atmos-config-digest_core/admin", buildConfigDigestKey("core/admin", ""))
}

// TestRetrieveWithConfigDigest_BackendError verifies a backend read failure is reported and is
// not mistaken for a missing entry.
func TestRetrieveWithConfigDigest_BackendError(t *testing.T) {
	kr := keyring.NewMockKeyring(gomock.NewController(t))
	store := &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeSystemKeyring}
	kr.EXPECT().Get(buildKeyringKey("mid", "acme")).Return("", assert.AnError)

	_, _, err := store.RetrieveWithConfigDigest("mid", "acme")
	require.ErrorIs(t, err, assert.AnError)
	require.ErrorIs(t, err, ErrCredentialStore)
	assert.NotErrorIs(t, err, ErrCredentialsNotFound)
}
