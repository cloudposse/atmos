package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const testRealm = "test-realm"

func TestGetGCPBaseDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")

	base, err := GetGCPBaseDir()
	require.NoError(t, err)
	assert.Contains(t, base, "atmos")
	assert.Equal(t, filepath.Join(tmp, "atmos"), base)
}

func TestGetProviderDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetProviderDir_EmptyRealm_UsesLegacyPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := GetProviderDir("", "gcp-adc")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", GCPSubdir, "gcp-adc")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
}

func TestGetProviderDir_InvalidName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetProviderDir(testRealm, "bad/name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestGetADCDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetADCDir(testRealm, providerName, "my-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "my-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetADCDir_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	_, err := GetADCDir(testRealm, providerName, "../id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestGetADCFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := GetADCFilePath(testRealm, providerName, "dev")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "dev", CredentialsFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestGetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetConfigDir(testRealm, providerName, "prod-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ConfigSubdir, "prod-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetPropertiesFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := GetPropertiesFilePath(testRealm, providerName, "test-id")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ConfigSubdir, "test-id", PropertiesFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestWriteADCFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	content := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.test-token",
		TokenExpiry: "2025-12-31T23:59:59Z",
	}
	path, err := WriteADCFile(testRealm, providerName, "adc-identity", content)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "application_default_credentials.json")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed AuthorizedUserContent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "authorized_user", parsed.Type)
	assert.Equal(t, "ya29.test-token", parsed.AccessToken)
	assert.Equal(t, "2025-12-31T23:59:59Z", parsed.TokenExpiry)

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWriteADCFile_NilContent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := WriteADCFile(testRealm, "gcp-adc", "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestWriteADCFile_PathResolutionErrorPreservesSentinels(t *testing.T) {
	tmp := t.TempDir()
	blockedConfigHome := filepath.Join(tmp, "blocked-config-home")
	require.NoError(t, os.WriteFile(blockedConfigHome, []byte("not a directory"), 0o600))
	t.Setenv("XDG_CONFIG_HOME", blockedConfigHome)

	_, err := WriteADCFile(testRealm, "gcp-adc", "id", &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.token",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWriteADCFile), "error should match ADC write sentinel")
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "error should preserve underlying auth config sentinel")
}

func TestWritePropertiesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := WritePropertiesFile(testRealm, providerName, "props-id", "my-project", "us-central1")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "[core]")
	assert.Contains(t, content, "project = my-project")
	assert.Contains(t, content, "[compute]")
	assert.Contains(t, content, "region = us-central1")

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWritePropertiesFile_EmptyProjectRegion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := WritePropertiesFile(testRealm, providerName, "empty-id", "", "")
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[core]")
	assert.Contains(t, string(data), "[compute]")
	assert.NotEmpty(t, path)
}

func TestWriteAccessTokenFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	expiry := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	path, err := WriteAccessTokenFile(testRealm, providerName, "token-id", "ya29.access-token", expiry)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := string(data)
	assert.Contains(t, lines, "ya29.access-token")
	assert.Contains(t, lines, "2025-06-15")
}

func TestCleanupIdentityFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	_, err := WriteADCFile(testRealm, providerName, "cleanup-id", &AuthorizedUserContent{Type: "authorized_user", AccessToken: "x"})
	require.NoError(t, err)
	_, err = WritePropertiesFile(testRealm, providerName, "cleanup-id", "p", "r")
	require.NoError(t, err)

	adcPath, _ := GetADCFilePath(testRealm, providerName, "cleanup-id")
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	err = CleanupIdentityFiles(testRealm, providerName, "cleanup-id")
	require.NoError(t, err)

	_, err = os.Stat(adcPath)
	require.True(t, os.IsNotExist(err))

	base, _ := GetGCPBaseDir()
	adcDir := filepath.Join(base, testRealm, GCPSubdir, providerName, ADCSubdir, "cleanup-id")
	configDir := filepath.Join(base, testRealm, GCPSubdir, providerName, ConfigSubdir, "cleanup-id")
	_, err = os.Stat(adcDir)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(configDir)
	require.True(t, os.IsNotExist(err))
}

func TestCleanupIdentityFiles_Nonexistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := CleanupIdentityFiles(testRealm, "gcp-adc", "nonexistent-identity")
	require.NoError(t, err)
}

func TestWriteAccessTokenFile_EmptyToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := WriteAccessTokenFile(testRealm, "gcp-adc", "empty-token-id", "", time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access token cannot be empty")
}

func TestWriteAccessTokenFile_ZeroExpiry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := WriteAccessTokenFile(testRealm, "gcp-adc", "zero-expiry-id", "ya29.token", time.Time{})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "ya29.token")
	// Zero expiry should not write a second line with timestamp.
	assert.Equal(t, "ya29.token\n", content)
}

func TestWritePropertiesFile_SpecialCharacters(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Test that ini.v1 properly handles special characters in values.
	path, err := WritePropertiesFile(testRealm, "gcp-adc", "special-id", "my-project-with-dashes_123", "us-east1")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "my-project-with-dashes_123")
	assert.Contains(t, content, "us-east1")
}

func TestWritePropertiesFile_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("File permission tests not reliable on Windows")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := WritePropertiesFile(testRealm, "gcp-adc", "perm-id", "proj", "region")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteADCFile_Overwrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Write first version.
	content1 := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "first-token",
	}
	path1, err := WriteADCFile(testRealm, "gcp-adc", "overwrite-id", content1)
	require.NoError(t, err)

	// Write second version to same identity.
	content2 := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "second-token",
	}
	path2, err := WriteADCFile(testRealm, "gcp-adc", "overwrite-id", content2)
	require.NoError(t, err)
	assert.Equal(t, path1, path2)

	// Verify second version is written.
	data, err := os.ReadFile(path2)
	require.NoError(t, err)
	var parsed AuthorizedUserContent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "second-token", parsed.AccessToken)
}

func TestWriteADCFile_ConcurrentReadersNeverSeePartialJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows atomic write fallback can briefly remove the target before rename")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	const (
		providerName = "gcp-adc"
		identityName = "concurrent-id"
		writers      = 4
		writes       = 8
		readers      = 8
		reads        = writers * writes * 4
	)

	_, err := WriteADCFile(testRealm, providerName, identityName, &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "seed-token",
		TokenExpiry: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)

	start := make(chan struct{})
	errCh := make(chan error, writers+readers)
	var wg sync.WaitGroup

	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			<-start
			for i := 0; i < writes; i++ {
				token := strings.Repeat("x", 64*1024) + string(rune('a'+writerID)) + string(rune('0'+i))
				_, writeErr := WriteADCFile(testRealm, providerName, identityName, &AuthorizedUserContent{
					Type:        "authorized_user",
					AccessToken: token,
					TokenExpiry: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				})
				if writeErr != nil {
					errCh <- writeErr
					return
				}
			}
		}(writer)
	}

	for reader := 0; reader < readers; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < reads; i++ {
				creds, readErr := LoadCredentialsFromFiles(context.Background(), testRealm, providerName, identityName)
				if readErr != nil {
					errCh <- readErr
					return
				}
				if creds == nil || creds.AccessToken == "" {
					errCh <- assert.AnError
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestGetConfigDir_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetConfigDir(testRealm, "gcp-adc", "..")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be")
}

func TestGetConfigDir_EmptyIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetConfigDir(testRealm, "gcp-adc", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity name is required")
}

func TestGetAccessTokenFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := GetAccessTokenFilePath(testRealm, "gcp-adc", "token-path-id")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, "gcp-adc", ADCSubdir, "token-path-id", AccessTokenFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestCleanupIdentityFiles_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := CleanupIdentityFiles(testRealm, "gcp-adc", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity name is required")
}

func TestWithFileLock_ReturnsErrCacheLocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file locking is a no-op fallback and never returns ErrCacheLocked")
	}

	tmp := t.TempDir()
	// Point the lock at a path whose parent directory does not exist, so the
	// underlying flock's O_CREATE open fails immediately without needing to
	// wait on real lock contention. Mirrors the established style in
	// pkg/cache/filelock_unix_test.go's TestWithLock_InvalidLockPath.
	path := filepath.Join(tmp, "nonexistent-dir", "target-file")

	called := false
	err := withFileLock(path, func() error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCacheLocked), "error should match cache locked sentinel")
	assert.False(t, called, "fn should not have run when the lock could not be acquired")
}

// skipIfCannotDenyDirWrite skips tests that rely on removing write permission
// from a directory to force a file-lock acquisition failure: this trick is a
// no-op on Windows (permissions work differently) and on Unix when running as
// root (root bypasses permission checks).
func skipIfCannotDenyDirWrite(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows file locking is a no-op fallback and never returns ErrCacheLocked")
	}
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
}

func TestWriteADCFile_LockFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "lock-fail-adc"

	// WriteADCFile locks a single per-identity target directly under providerDir
	// (shared with the other write helpers and CleanupIdentityFiles), not a file
	// next to the ADC file itself. Strip write permission from providerDir so that
	// shared lock's sibling ".lock" file cannot be created, forcing a
	// lock-acquisition failure without any real lock contention.
	_, err := GetADCFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	providerDir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(providerDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(providerDir, 0o700) })

	_, err = WriteADCFile(testRealm, providerName, identityName, &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.token",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWriteADCFile), "error should match ADC write sentinel")
}

func TestWritePropertiesFile_LockFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "lock-fail-props"

	// See TestWriteADCFile_LockFailure_WrapsSentinel: the shared per-identity lock
	// lives directly under providerDir, so that's what must be made read-only.
	_, err := GetPropertiesFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	providerDir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(providerDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(providerDir, 0o700) })

	_, err = WritePropertiesFile(testRealm, providerName, identityName, "my-project", "us-central1")

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWritePropertiesFile), "error should match properties write sentinel")
}

func TestWriteAccessTokenFile_LockFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "lock-fail-token"

	// See TestWriteADCFile_LockFailure_WrapsSentinel: the shared per-identity lock
	// lives directly under providerDir, so that's what must be made read-only.
	_, err := GetAccessTokenFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	providerDir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(providerDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(providerDir, 0o700) })

	_, err = WriteAccessTokenFile(testRealm, providerName, identityName, "ya29.access-token", time.Time{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWriteAccessTokenFile), "error should match access token write sentinel")
}

func TestCleanupIdentityFiles_LockFailure_WrapsError(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "lock-fail-cleanup"

	// CleanupIdentityFiles locks the same shared per-identity target as the write
	// helpers (providerDir/<identity>.identity), a marker that never exists on disk,
	// so its sibling ".lock" file lives directly under providerDir. Deny write
	// access on providerDir itself.
	providerDir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(providerDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(providerDir, 0o700) })

	err = CleanupIdentityFiles(testRealm, providerName, identityName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup identity files", "error should be wrapped with the cleanup context")
	assert.True(t, errors.Is(err, errUtils.ErrCacheLocked), "error should preserve the underlying cache-locked sentinel")
}

// TestWriteADCFile_WriteFailure_WrapsSentinel verifies the WriteFileAtomic
// failure branch *inside* the lock closure (distinct from the lock-acquisition
// failure covered by TestWriteADCFile_LockFailure_WrapsSentinel above): the
// sibling ".lock" file is pre-created so locking succeeds, then the directory
// is made read-only so renameio's temp-file-plus-rename write fails.
func TestWriteADCFile_WriteFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "write-fail-adc"

	path, err := GetADCFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	dir := filepath.Dir(path)
	require.NoError(t, os.WriteFile(path+".lock", nil, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err = WriteADCFile(testRealm, providerName, identityName, &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.token",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWriteADCFile), "error should match ADC write sentinel")
}

// TestWritePropertiesFile_WriteFailure_WrapsSentinel mirrors the ADC case for
// WritePropertiesFile's WriteFileAtomic failure branch.
func TestWritePropertiesFile_WriteFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "write-fail-props"

	path, err := GetPropertiesFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	dir := filepath.Dir(path)
	require.NoError(t, os.WriteFile(path+".lock", nil, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err = WritePropertiesFile(testRealm, providerName, identityName, "my-project", "us-central1")

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWritePropertiesFile), "error should match properties write sentinel")
}

// TestWriteAccessTokenFile_WriteFailure_WrapsSentinel mirrors the ADC case for
// WriteAccessTokenFile's WriteFileAtomic failure branch.
func TestWriteAccessTokenFile_WriteFailure_WrapsSentinel(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "write-fail-token"

	path, err := GetAccessTokenFilePath(testRealm, providerName, identityName)
	require.NoError(t, err)
	dir := filepath.Dir(path)
	require.NoError(t, os.WriteFile(path+".lock", nil, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err = WriteAccessTokenFile(testRealm, providerName, identityName, "ya29.access-token", time.Time{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWriteAccessTokenFile), "error should match access token write sentinel")
}

// TestCleanupIdentityFiles_RemoveAllFailure_JoinsErrors verifies that an
// os.RemoveAll failure for either the ADC or config subdirectory (a
// non-ENOENT error) is preserved and joined rather than silently ignored:
// both subdirectories get a file inside them so RemoveAll must unlink it, then
// are made read-only so the unlink fails with permission denied.
func TestCleanupIdentityFiles_RemoveAllFailure_JoinsErrors(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "cleanup-removeall-fail"

	adcDir, err := GetADCDir(testRealm, providerName, identityName)
	require.NoError(t, err)
	configDir, err := GetConfigDir(testRealm, providerName, identityName)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(adcDir, "cred.json"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "properties"), []byte(""), 0o600))
	require.NoError(t, os.Chmod(adcDir, 0o555))
	require.NoError(t, os.Chmod(configDir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(adcDir, 0o700)
		_ = os.Chmod(configDir, 0o700)
	})

	err = CleanupIdentityFiles(testRealm, providerName, identityName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRemoveDirectory), "error should wrap the remove-directory sentinel")
	// Both directory-specific failures should be present in the joined error,
	// not just whichever one happened to be appended first.
	assert.Contains(t, err.Error(), adcDir, "joined error should mention the ADC directory that failed to remove")
	assert.Contains(t, err.Error(), configDir, "joined error should mention the config directory that failed to remove")
}

// TestIdentityLockTarget_SharedAcrossWritesAndCleanup verifies that
// WriteADCFile, WritePropertiesFile, WriteAccessTokenFile, and
// CleanupIdentityFiles all resolve to the identical lock target for a given
// (realm, provider, identity), which is what makes them actually contend with
// each other instead of racing (see identityLockTarget's doc comment).
func TestIdentityLockTarget_SharedAcrossWritesAndCleanup(t *testing.T) {
	providerDir := filepath.Join(t.TempDir(), "provider")
	identityName := "shared-lock-identity"

	target := identityLockTarget(providerDir, identityName)
	assert.Equal(t, target, identityLockTarget(providerDir, identityName), "must be deterministic")
	assert.Equal(t, filepath.Join(providerDir, identityName+".identity"), target)
	// Must live directly under providerDir, not inside a subdirectory CleanupIdentityFiles removes.
	assert.Equal(t, providerDir, filepath.Dir(target))
}

// TestCleanupIdentityFiles_ContendsWithConcurrentWrite proves cleanup and a
// write now share a single lock: while an external holder occupies the
// identity lock, CleanupIdentityFiles must block rather than proceed
// concurrently (which would previously let it RemoveAll a directory a write
// was still populating), and must complete only after the lock is released.
func TestCleanupIdentityFiles_ContendsWithConcurrentWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file locking is best-effort/noop on Windows; contention is not observable there")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"
	identityName := "contended-identity"

	providerDir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)

	// Hold the shared identity lock externally, simulating a write in progress.
	held := flock.New(identityLockTarget(providerDir, identityName) + ".lock")
	locked, err := held.TryLock()
	require.NoError(t, err)
	require.True(t, locked, "external holder should acquire the identity lock")

	done := make(chan error, 1)
	go func() {
		done <- CleanupIdentityFiles(testRealm, providerName, identityName)
	}()

	// CleanupIdentityFiles must still be blocked shortly after starting: it
	// cannot proceed while the identity lock is held elsewhere.
	select {
	case err := <-done:
		t.Fatalf("CleanupIdentityFiles returned early (err=%v) while the identity lock was still held; cleanup and writes are not contending on the same lock", err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, held.Unlock())

	select {
	case err := <-done:
		assert.NoError(t, err, "cleanup should succeed once the lock is released")
	case <-time.After(fileLockTimeout):
		t.Fatal("CleanupIdentityFiles did not complete after the identity lock was released")
	}
}

func TestValidatePathSegment(t *testing.T) {
	tests := []struct {
		name      string
		label     string
		value     string
		wantErr   bool
		errSubstr string
	}{
		{name: "valid segment", label: "test", value: "valid-name", wantErr: false},
		{name: "empty value", label: "test", value: "", wantErr: true, errSubstr: "is required"},
		{name: "dot segment", label: "test", value: ".", wantErr: true, errSubstr: "must not be"},
		{name: "dotdot segment", label: "test", value: "..", wantErr: true, errSubstr: "must not be"},
		{name: "forward slash", label: "test", value: "a/b", wantErr: true, errSubstr: "path separators"},
		{name: "backslash", label: "test", value: "a\\b", wantErr: true, errSubstr: "path separators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSegment(tt.label, tt.value)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
