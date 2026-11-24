package credentials

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExternalConfigDigest_PersistentBackendsNeverTouchTheKeyring reproduces the 1.235.0 upgrade
// regression where `atmos auth login` wrote an `atmos-config-digest_<realm>__<identity>` keyring
// entry for identities whose credentials never live in the keyring (for example aws/user with
// inline keys, whose session credentials are in files). On macOS a new Keychain item can prompt
// (a login hung for minutes), and the file keyring asks for its password. The record describes
// files, holds nothing secret, and now lives in a file next to the keyring, so the keyring is
// never read or written for it. The mocked keyring has no expectations: any call fails the test.
func TestExternalConfigDigest_PersistentBackendsNeverTouchTheKeyring(t *testing.T) {
	for _, storeType := range []string{types.CredentialStoreTypeSystemKeyring, types.CredentialStoreTypeFile} {
		t.Run(storeType, func(t *testing.T) {
			kr := keyring.NewMockKeyring(gomock.NewController(t))
			dir := filepath.Join(t.TempDir(), "config-digests")
			store := &keyringCredentialStore{kr: kr, storeType: storeType, digestDir: dir}
			record := ExternalConfigDigest{Digest: "abc123", Expiration: "2026-11-22T12:00:00Z", CredentialID: "ASIAEXAMPLE"}

			require.NoError(t, store.StoreExternalConfigDigest("core/terraform:admin", "acme", record))
			got, err := store.RetrieveExternalConfigDigest("core/terraform:admin", "acme")
			require.NoError(t, err)
			assert.Equal(t, record, got)

			_, err = store.RetrieveExternalConfigDigest("core/terraform:admin", "other")
			require.ErrorIs(t, err, ErrDataNotFound, "records are realm-scoped")

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "one record file, whatever characters the identity name has")
			info, err := entries[0].Info()
			require.NoError(t, err)
			assert.True(t, info.Mode().IsRegular())

			require.NoError(t, store.DeleteExternalConfigDigest("core/terraform:admin", "acme"))
			require.NoError(t, store.DeleteExternalConfigDigest("core/terraform:admin", "acme"), "delete is idempotent")
			_, err = store.RetrieveExternalConfigDigest("core/terraform:admin", "acme")
			require.ErrorIs(t, err, ErrDataNotFound)
		})
	}
}

// TestExternalConfigDigest_FileRecordErrors verifies a corrupt record and an unwritable record
// directory are reported, never mistaken for a missing record.
func TestExternalConfigDigest_FileRecordErrors(t *testing.T) {
	t.Run("corrupt record", func(t *testing.T) {
		store := &keyringCredentialStore{storeType: types.CredentialStoreTypeFile, digestDir: t.TempDir()}
		require.NoError(t, store.StoreExternalConfigDigest("mid", "acme", ExternalConfigDigest{Digest: "d"}))
		require.NoError(t, os.WriteFile(store.configDigestFile("mid", "acme"), []byte("not json"), 0o600))

		_, err := store.RetrieveExternalConfigDigest("mid", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		assert.NotErrorIs(t, err, ErrDataNotFound)
	})

	t.Run("record directory is a file", func(t *testing.T) {
		parent := t.TempDir()
		blocker := filepath.Join(parent, "config-digests")
		require.NoError(t, os.WriteFile(blocker, nil, 0o600))
		store := &keyringCredentialStore{storeType: types.CredentialStoreTypeFile, digestDir: blocker}

		err := store.StoreExternalConfigDigest("mid", "acme", ExternalConfigDigest{Digest: "d"})
		require.ErrorIs(t, err, ErrCredentialStore)
		_, err = store.RetrieveExternalConfigDigest("mid", "acme")
		require.ErrorIs(t, err, ErrCredentialStore)
		assert.NotErrorIs(t, err, ErrDataNotFound)
		require.ErrorIs(t, store.DeleteExternalConfigDigest("mid", "acme"), ErrCredentialStore)
	})
}

// TestExternalConfigDigest_DeleteRemovesFileRecord verifies Delete (used by keychain logout)
// removes the file record together with the credentials.
func TestExternalConfigDigest_DeleteRemovesFileRecord(t *testing.T) {
	store := newMemoryKeyringStore()
	store.digestDir = t.TempDir()
	require.NoError(t, store.Store("mid", &types.OIDCCredentials{Token: "t"}, "acme"))
	require.NoError(t, store.StoreExternalConfigDigest("mid", "acme", ExternalConfigDigest{Digest: "d"}))

	require.NoError(t, store.Delete("mid", "acme"))
	_, err := store.RetrieveExternalConfigDigest("mid", "acme")
	require.ErrorIs(t, err, ErrDataNotFound)
}

// TestPersistentStoresKeepDigestRecordsOutsideTheKeyring verifies the production constructors
// place the records outside the keyring: the file backend next to its keyring directory, so a
// custom keyring path keeps its records with it, and the system backend in the Atmos cache.
func TestPersistentStoresKeepDigestRecordsOutsideTheKeyring(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "keyring")
		t.Setenv("ATMOS_KEYRING_PASSWORD", "atmos-test-keyring-password")
		t.Setenv("ATMOS_KEYRING_FILE_PATH", "")
		store, err := newFileKeyringStore(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "file", Spec: map[string]interface{}{"path": dir}}})
		require.NoError(t, err)
		assert.Equal(t, dir+configDigestDirSuffix, store.digestDir)
	})

	t.Run("system", func(t *testing.T) {
		cache := t.TempDir()
		t.Setenv("ATMOS_XDG_CACHE_HOME", cache)
		assert.Equal(t, filepath.Join(cache, "atmos", "auth", "config-digests"), systemConfigDigestDir())
	})

	t.Run("memory", func(t *testing.T) {
		assert.Empty(t, newMemoryKeyringStore().digestDir, "the memory backend keeps records in memory")
	})
}

// TestExternalConfigDigest_FileRecordWriteFailure verifies a record that cannot be written (here
// a directory occupies the record's path) is reported.
func TestExternalConfigDigest_FileRecordWriteFailure(t *testing.T) {
	store := &keyringCredentialStore{storeType: types.CredentialStoreTypeFile, digestDir: t.TempDir()}
	require.NoError(t, os.MkdirAll(filepath.Join(store.configDigestFile("mid", "acme"), "child"), 0o700))

	err := store.StoreExternalConfigDigest("mid", "acme", ExternalConfigDigest{Digest: "d"})
	require.ErrorIs(t, err, ErrCredentialStore)
}

// TestRecordIsMissing verifies how a "not exist" answer for a record file is classified on every
// platform: missing when the record directory is absent or is a directory, a store failure when a
// non-directory occupies it (Windows reports that case as "path not found").
func TestRecordIsMissing(t *testing.T) {
	parent := t.TempDir()
	notExist := &os.PathError{Op: "open", Path: "record.json", Err: fs.ErrNotExist}

	absent := &keyringCredentialStore{digestDir: filepath.Join(parent, "absent")}
	assert.True(t, absent.recordIsMissing(notExist))

	dir := &keyringCredentialStore{digestDir: parent}
	assert.True(t, dir.recordIsMissing(notExist))

	blocker := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	blocked := &keyringCredentialStore{digestDir: blocker}
	assert.False(t, blocked.recordIsMissing(notExist), "a file in place of the record directory is a store failure")

	assert.False(t, dir.recordIsMissing(fs.ErrPermission), "other errors are never a missing record")
}
