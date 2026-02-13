package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRealmMismatchFiles_NonEmptyRealm_FindsNoRealmCreds(t *testing.T) {
	// Setup: credentials at {baseDir}/aws/{provider}/credentials (no realm).
	baseDir := t.TempDir()
	providerDir := filepath.Join(baseDir, "aws", "my-provider")
	require.NoError(t, os.MkdirAll(providerDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(providerDir, "credentials"), []byte("[default]\naws_access_key_id=AKIA\n"), 0o600))

	// Override XDG to point to our temp dir.
	t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Dir(baseDir))
	// The XDG function appends "atmos" to the path, so set parent.
	// We need baseDir to be {parent}/atmos.
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

func TestHasCredentialFiles_Positive(t *testing.T) {
	// Setup: {awsDir}/{provider}/credentials exists.
	awsDir := filepath.Join(t.TempDir(), "aws")
	providerDir := filepath.Join(awsDir, "my-provider")
	require.NoError(t, os.MkdirAll(providerDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(providerDir, "credentials"), []byte("test"), 0o600))

	assert.True(t, hasCredentialFiles(awsDir))
}

func TestHasCredentialFiles_NoProviderDirs(t *testing.T) {
	// Setup: aws dir exists but no provider subdirectories.
	awsDir := filepath.Join(t.TempDir(), "aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))

	assert.False(t, hasCredentialFiles(awsDir))
}

func TestHasCredentialFiles_NoCredentialsFile(t *testing.T) {
	// Setup: provider dir exists but no credentials file.
	awsDir := filepath.Join(t.TempDir(), "aws")
	providerDir := filepath.Join(awsDir, "my-provider")
	require.NoError(t, os.MkdirAll(providerDir, 0o700))

	assert.False(t, hasCredentialFiles(awsDir))
}

func TestHasCredentialFiles_NonexistentDir(t *testing.T) {
	assert.False(t, hasCredentialFiles(filepath.Join(t.TempDir(), "nonexistent")))
}

func TestLogRealmMismatchWarning_NonEmptyToEmpty(t *testing.T) {
	// Just verify it doesn't panic. The actual log output goes to the logger.
	logRealmMismatchWarning("my-realm", "(no realm)")
}

func TestLogRealmMismatchWarning_EmptyToNonEmpty(t *testing.T) {
	// Just verify it doesn't panic.
	logRealmMismatchWarning("", "my-project")
}

func TestRealmMismatchWarningOnce_OnlyFiresOnce(t *testing.T) {
	// Reset the once for testing.
	realmMismatchWarningOnce = sync.Once{}

	callCount := 0
	realmMismatchWarningOnce.Do(func() {
		callCount++
	})
	// Second call should be no-op.
	realmMismatchWarningOnce.Do(func() {
		callCount++
	})
	assert.Equal(t, 1, callCount)
}
