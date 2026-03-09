package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
)

func TestCheckRealmMismatchFiles_NonEmptyRealm_FindsNoRealmCreds(t *testing.T) {
	// Setup: credentials at {parent}/atmos/aws/{provider}/credentials (no realm).
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(filepath.Join(atmosDir, "aws", "my-provider"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, "aws", "my-provider", "credentials"), []byte("[default]\naws_access_key_id=AKIA\n"), 0o600))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	result := checkRealmMismatchFiles("my-realm")
	assert.Equal(t, "(no realm)", result)
}

func TestCheckRealmMismatchFiles_EmptyRealm_FindsRealmScopedCreds(t *testing.T) {
	// Setup: credentials at {baseDir}/{realm}/aws/{provider}/credentials.
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	realmDir := filepath.Join(atmosDir, "my-project", "aws", "github-oidc")
	require.NoError(t, os.MkdirAll(realmDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(realmDir, "credentials"), []byte("[default]\naws_access_key_id=AKIA\n"), 0o600))

	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	result := checkRealmMismatchFiles("")
	assert.Equal(t, "my-project", result)
}

func TestCheckRealmMismatchFiles_NoMismatch(t *testing.T) {
	// Setup: empty base directory with no credentials anywhere.
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(atmosDir, 0o700))

	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	result := checkRealmMismatchFiles("")
	assert.Empty(t, result)

	result = checkRealmMismatchFiles("some-realm")
	assert.Empty(t, result)
}

func TestCheckRealmMismatchFiles_NonexistentBaseDir(t *testing.T) {
	// Point to a non-existent directory — os.Stat should fail, returning "".
	parent := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	assert.Empty(t, checkRealmMismatchFiles("my-realm"))
	assert.Empty(t, checkRealmMismatchFiles(""))
}

func TestHasCredentialFiles(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // returns awsDir path.
		expected bool
	}{
		{
			name: "credentials file exists under provider",
			setup: func(t *testing.T) string {
				awsDir := filepath.Join(t.TempDir(), "aws")
				providerDir := filepath.Join(awsDir, "my-provider")
				require.NoError(t, os.MkdirAll(providerDir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(providerDir, "credentials"), []byte("test"), 0o600))
				return awsDir
			},
			expected: true,
		},
		{
			name: "aws dir exists but no provider subdirectories",
			setup: func(t *testing.T) string {
				awsDir := filepath.Join(t.TempDir(), "aws")
				require.NoError(t, os.MkdirAll(awsDir, 0o700))
				return awsDir
			},
			expected: false,
		},
		{
			name: "provider dir exists but no credentials file",
			setup: func(t *testing.T) string {
				awsDir := filepath.Join(t.TempDir(), "aws")
				providerDir := filepath.Join(awsDir, "my-provider")
				require.NoError(t, os.MkdirAll(providerDir, 0o700))
				return awsDir
			},
			expected: false,
		},
		{
			name: "nonexistent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expected: false,
		},
		{
			name: "skips non-directory entries in aws dir",
			setup: func(t *testing.T) string {
				awsDir := filepath.Join(t.TempDir(), "aws")
				require.NoError(t, os.MkdirAll(awsDir, 0o700))
				// Create a regular file (not a directory) inside the aws dir.
				require.NoError(t, os.WriteFile(filepath.Join(awsDir, "stray-file.txt"), []byte("not a dir"), 0o600))
				return awsDir
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			awsDir := tt.setup(t)
			assert.Equal(t, tt.expected, hasCredentialFiles(awsDir))
		})
	}
}

func TestScanForRealmCredentials_SkipsFilesAndAwsDir(t *testing.T) {
	// Setup: base dir with a file, an "aws" dir, and a realm dir.
	parent := t.TempDir()
	baseDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(baseDir, 0o700))

	// Create a regular file in baseDir (should be skipped by !entry.IsDir()).
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("test"), 0o600))

	// Create an "aws" directory (should be skipped by name == awsDirNameForMismatch).
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "aws", "provider"), 0o700))

	// Create a realm dir without credentials (should be checked but return "").
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "empty-realm"), 0o700))

	result := scanForRealmCredentials(baseDir)
	assert.Empty(t, result, "should not find any realm credentials")
}

func TestScanForRealmCredentials_NonexistentDir(t *testing.T) {
	result := scanForRealmCredentials(filepath.Join(t.TempDir(), "nonexistent"))
	assert.Empty(t, result)
}

func TestCheckRealmMismatchKeyring_NilStore(t *testing.T) {
	m := &manager{credentialStore: nil}
	result := m.checkRealmMismatchKeyring("my-identity", "my-realm")
	assert.Empty(t, result, "should return empty when credentialStore is nil")
}

func TestCheckRealmMismatchKeyring_EmptyCurrentRealm(t *testing.T) {
	// When current realm is empty, we can't probe keyring without listing.
	m := &manager{credentialStore: &testStore{}}
	result := m.checkRealmMismatchKeyring("my-identity", "")
	assert.Empty(t, result, "should return empty when current realm is empty")
}

func TestCheckRealmMismatchKeyring_FindsCredsUnderNoRealm(t *testing.T) {
	// Store has credentials under the empty realm for the identity.
	store := &testStore{data: map[string]any{
		"my-identity": &testCreds{},
	}}
	m := &manager{credentialStore: store}
	result := m.checkRealmMismatchKeyring("my-identity", "my-realm")
	assert.Equal(t, "(no realm)", result, "should detect credentials under empty realm")
}

func TestCheckRealmMismatchKeyring_NoCredsAnywhere(t *testing.T) {
	// Store has no credentials at all.
	store := &testStore{data: map[string]any{}}
	m := &manager{credentialStore: store}
	result := m.checkRealmMismatchKeyring("my-identity", "my-realm")
	assert.Empty(t, result, "should return empty when no credentials exist")
}

func TestEmitRealmMismatchWarning_KeyringDetection(t *testing.T) {
	// Reset the sync.Once for this test.
	realmMismatchWarningOnce = sync.Once{}

	store := &testStore{data: map[string]any{
		"my-identity": &testCreds{},
	}}
	m := &manager{
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "my-realm", Source: "config"},
	}

	// Should not panic and should detect the mismatch via keyring.
	m.emitRealmMismatchWarning("my-identity")
}

func TestEmitRealmMismatchWarning_FileDetection(t *testing.T) {
	// Reset the sync.Once for this test.
	realmMismatchWarningOnce = sync.Once{}

	// Setup file-based credentials under no-realm path.
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(filepath.Join(atmosDir, "aws", "my-provider"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, "aws", "my-provider", "credentials"), []byte("test"), 0o600))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	// Empty credential store — keyring probe won't find anything, so falls through to file check.
	store := &testStore{data: map[string]any{}}
	m := &manager{
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "my-realm", Source: "config"},
	}

	m.emitRealmMismatchWarning("my-identity")
}

func TestEmitRealmMismatchWarning_NoMismatch(t *testing.T) {
	// Reset the sync.Once for this test.
	realmMismatchWarningOnce = sync.Once{}

	// No credentials anywhere — neither keyring nor files.
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(atmosDir, 0o700))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	store := &testStore{data: map[string]any{}}
	m := &manager{
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "my-realm", Source: "config"},
	}

	// Should not panic, should not warn.
	m.emitRealmMismatchWarning("my-identity")
}

func TestEmitRealmMismatchWarning_EmptyRealm(t *testing.T) {
	// Reset the sync.Once for this test.
	realmMismatchWarningOnce = sync.Once{}

	// With empty realm, keyring probe is skipped. File detection finds realm-scoped creds.
	parent := t.TempDir()
	atmosDir := filepath.Join(parent, "atmos")
	require.NoError(t, os.MkdirAll(filepath.Join(atmosDir, "some-realm", "aws", "provider"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, "some-realm", "aws", "provider", "credentials"), []byte("test"), 0o600))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", parent)

	store := &testStore{data: map[string]any{}}
	m := &manager{
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "", Source: "auto"},
	}

	m.emitRealmMismatchWarning("my-identity")
}

func TestEmitRealmMismatchWarning_OnlyFiresOnce(t *testing.T) {
	// Reset the sync.Once for this test.
	realmMismatchWarningOnce = sync.Once{}

	store := &testStore{data: map[string]any{
		"id1": &testCreds{},
	}}
	m := &manager{
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "my-realm", Source: "config"},
	}

	// Call twice — the sync.Once ensures the check only runs once.
	m.emitRealmMismatchWarning("id1")
	m.emitRealmMismatchWarning("id1")
	// No assertion needed — we're verifying it doesn't panic on second call.
}

func TestCheckNoRealmCredentials(t *testing.T) {
	t.Run("finds credentials", func(t *testing.T) {
		baseDir := t.TempDir()
		awsDir := filepath.Join(baseDir, "aws", "provider")
		require.NoError(t, os.MkdirAll(awsDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(awsDir, "credentials"), []byte("test"), 0o600))
		assert.Equal(t, "(no realm)", checkNoRealmCredentials(baseDir))
	})

	t.Run("no credentials", func(t *testing.T) {
		baseDir := t.TempDir()
		assert.Empty(t, checkNoRealmCredentials(baseDir))
	})
}

func TestLogRealmMismatchWarning_NonEmptyToEmpty(t *testing.T) {
	// Verify it doesn't panic. The actual log output goes to the logger.
	logRealmMismatchWarning("my-realm", "(no realm)")
}

func TestLogRealmMismatchWarning_EmptyToNonEmpty(t *testing.T) {
	// Verify it doesn't panic.
	logRealmMismatchWarning("", "my-project")
}
