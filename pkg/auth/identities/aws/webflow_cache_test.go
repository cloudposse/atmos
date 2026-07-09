package aws

// Tests for refresh-token XDG cache file I/O (webflow_cache.go).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRefreshCache_SaveAndLoad(t *testing.T) {
	// Use a temp directory for cache.
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test-user",
		realm: "test-realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	expiresAt := time.Now().Add(12 * time.Hour).Truncate(time.Second)

	// Save refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "my-refresh-token",
		Region:       "us-east-2",
		ExpiresAt:    expiresAt,
	})

	// Load refresh cache.
	cache, err := identity.loadRefreshCache()
	require.NoError(t, err)
	assert.Equal(t, "my-refresh-token", cache.RefreshToken)
	assert.Equal(t, "us-east-2", cache.Region)
	// Time comparison with tolerance for JSON serialization.
	assert.WithinDuration(t, expiresAt, cache.ExpiresAt, time.Second)

	// Verify file path uses filepath.Join.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	assert.Contains(t, cachePath, filepath.Join("aws-webflow", "test-user-test-realm", "refresh.json"))
}

func TestRefreshCache_LoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "nonexistent",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	cache, err := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, err)
}

func TestRefreshCache_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save then delete.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "token",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	identity.deleteRefreshCache()

	// Should not be loadable after delete.
	cache, err := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, err)
}

func TestCacheFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix file permissions")
	}

	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "perm-test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "token",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)

	info, err := os.Stat(cachePath)
	require.NoError(t, err)
	// File should be readable/writable only by owner.
	assert.Equal(t, os.FileMode(webflowCacheFilePerms), info.Mode().Perm())
}

func TestLoadRefreshCache_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Write invalid JSON to the cache file.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	err = os.WriteFile(cachePath, []byte("not-json"), webflowCacheFilePerms)
	require.NoError(t, err)

	cache, loadErr := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "parse refresh cache")
}

func TestLoadRefreshCache_EmptyRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Write cache with empty refresh token.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	data, _ := json.Marshal(&webflowRefreshCache{
		RefreshToken: "",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	err = os.WriteFile(cachePath, data, webflowCacheFilePerms)
	require.NoError(t, err)

	cache, loadErr := identity.loadRefreshCache()
	assert.Nil(t, cache)
	require.Error(t, loadErr)
	assert.ErrorIs(t, loadErr, errUtils.ErrWebflowEmptyCachedToken)
}

func TestDeleteRefreshCache_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Should not panic when deleting non-existent cache.
	identity.deleteRefreshCache()
}

func TestGetRefreshCachePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "my-identity",
		realm: "my-realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	path, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	assert.Contains(t, path, filepath.Join("aws-webflow", "my-identity-my-realm", "refresh.json"))
}

// TestSaveRefreshCache_InvalidPath verifies saveRefreshCache logs and returns
// silently when the cache path cannot be resolved (exercises the error branch
// at webflow.go:694-697). We trigger this by pointing XDG_CACHE_HOME at a path
// that cannot be created (a file, not a directory).
func TestSaveRefreshCache_InvalidPath(t *testing.T) {
	// Create a file and use it as XDG_CACHE_HOME so MkdirAll fails.
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))
	t.Setenv("ATMOS_XDG_CACHE_HOME", blockingFile)

	identity := &userIdentity{
		name:   "test-save-bad-path",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Should not panic; logs and returns.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "x",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	})
}

// TestDeleteRefreshCache_InvalidPath verifies deleteRefreshCache returns
// silently when the cache path cannot be resolved.
func TestDeleteRefreshCache_InvalidPath(t *testing.T) {
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o600))
	t.Setenv("ATMOS_XDG_CACHE_HOME", blockingFile)

	identity := &userIdentity{
		name:   "test-delete-bad-path",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Should not panic.
	identity.deleteRefreshCache()
}

// TestGetRefreshCachePath_MkdirAllFailure verifies the inner MkdirAll
// failure branch at webflow_cache.go:25. Triggered by placing a file where
// the identity-scoped directory should be.
func TestGetRefreshCachePath_MkdirAllFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	// xdg.GetXDGCacheDir creates $tmpDir/atmos/aws-webflow/. Then our
	// MkdirAll tries atmos/aws-webflow/<name>-<realm>. Put a FILE at that
	// exact path so MkdirAll fails.
	awsWebflowDir := filepath.Join(tmpDir, "atmos", "aws-webflow")
	require.NoError(t, os.MkdirAll(awsWebflowDir, 0o700))
	blockingFile := filepath.Join(awsWebflowDir, "blocked-name-blocked-realm")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocker"), 0o600))

	identity := &userIdentity{
		name:   "blocked-name",
		realm:  "blocked-realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	path, err := identity.getRefreshCachePath()
	assert.Empty(t, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create cache directory")
}

// TestLoadRefreshCache_PathError verifies that loadRefreshCache surfaces the
// getRefreshCachePath error (the err-return branch at webflow_cache.go:35).
func TestLoadRefreshCache_PathError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	awsWebflowDir := filepath.Join(tmpDir, "atmos", "aws-webflow")
	require.NoError(t, os.MkdirAll(awsWebflowDir, 0o700))
	blockingFile := filepath.Join(awsWebflowDir, "load-name-load-realm")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocker"), 0o600))

	identity := &userIdentity{
		name:   "load-name",
		realm:  "load-realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	cache, err := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, err)
}

// TestSaveRefreshCache_WriteFileFailure verifies that saveRefreshCache logs
// and returns silently when os.WriteFile fails (the path exists as a
// directory rather than a writable file location). Exercises lines 70-73.
func TestSaveRefreshCache_WriteFileFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem permission semantics differ on Windows")
	}
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:   "writefile-name",
		realm:  "writefile-realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Force getRefreshCachePath to return a path that points at an existing
	// directory (not a file), so WriteFile fails with EISDIR. Pre-create the
	// directory structure and place a directory where the file should be.
	cacheFilePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	// Replace the file location with a directory.
	_ = os.Remove(cacheFilePath)
	require.NoError(t, os.MkdirAll(cacheFilePath, 0o700))

	// Should not panic.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "x",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
}

// TestDeleteRefreshCache_RemoveFailure verifies the non-IsNotExist error
// branch (webflow_cache.go:85). Triggered by placing a non-empty directory
// where the file should be, so os.Remove fails with "directory not empty".
func TestDeleteRefreshCache_RemoveFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem permission semantics differ on Windows")
	}
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:   "rm-name",
		realm:  "rm-realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	cacheFilePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	_ = os.Remove(cacheFilePath)
	require.NoError(t, os.MkdirAll(cacheFilePath, 0o700))
	// Put a file inside so removing the dir fails with ENOTEMPTY.
	require.NoError(t, os.WriteFile(filepath.Join(cacheFilePath, "keep"), []byte("x"), 0o600))

	// Should not panic.
	identity.deleteRefreshCache()
}
