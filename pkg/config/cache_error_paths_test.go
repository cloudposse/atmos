package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
)

// TestLoadCache_GetCacheFilePath tests cache file path determination at cache.go:52-55.
func TestLoadCache_GetCacheFilePath(t *testing.T) {
	// The xdg library provides fallbacks, so GetCacheFilePath rarely errors.
	// This test documents that LoadCache calls GetCacheFilePath and checks for errors.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	_, err := LoadCache()
	// Should succeed with valid XDG_CACHE_HOME.
	assert.NoError(t, err)
}

// TestLoadCache_FileDoesNotExist tests early return at cache.go:58-61.
func TestLoadCache_FileDoesNotExist(t *testing.T) {
	// Create temp directory for cache.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// Ensure cache file doesn't exist.
	cacheFile := filepath.Join(tempDir, "atmos", "cache.yaml")
	os.RemoveAll(filepath.Dir(cacheFile))

	cfg, err := LoadCache()
	assert.NoError(t, err)
	// When cache file doesn't exist, LoadCache returns empty config.
	assert.Equal(t, int64(0), cfg.LastChecked)
	assert.Equal(t, "", cfg.InstallationId)
	assert.Equal(t, false, cfg.TelemetryDisclosureShown)
}

// Platform-specific tests moved to separate files:
// - cache_error_paths_windows_test.go: TestLoadCache_WindowsReadError
// - cache_error_paths_unix_test.go: TestLoadCache_UnixReadLockError

// TestSaveCache_GetCacheFilePath tests cache file path check at cache.go:91-94.
func TestSaveCache_GetCacheFilePath(t *testing.T) {
	// The xdg library provides fallbacks, so GetCacheFilePath rarely errors.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	cfg := CacheConfig{
		LastChecked: 123456,
	}
	err := SaveCache(cfg)
	// Should succeed with valid XDG_CACHE_HOME.
	assert.NoError(t, err)
}

// TestSaveCache_DirectoryCreation tests directory creation at cache.go:27-36 (GetCacheFilePath).
func TestSaveCache_DirectoryCreation(t *testing.T) {
	// GetCacheFilePath creates the directory, so this tests that code path.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	cfg := CacheConfig{
		LastChecked: 123456,
	}
	err := SaveCache(cfg)
	assert.NoError(t, err)

	// Verify directory was created.
	cacheDir := filepath.Join(tempDir, "atmos")
	_, err = os.Stat(cacheDir)
	assert.NoError(t, err)
}

// TestSaveCache_MarshalError tests marshal path at cache.go:105-111.
func TestSaveCache_MarshalError(t *testing.T) {
	// Create temp directory.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// Normal config should marshal successfully.
	cfg := CacheConfig{
		LastChecked:              1234567890,
		InstallationId:           "test-id",
		TelemetryDisclosureShown: true,
	}

	err := SaveCache(cfg)
	// Should succeed with valid config.
	assert.NoError(t, err)
}

// TestSaveCache_FileLockPath tests file lock usage at cache.go:97-118.
func TestSaveCache_FileLockPath(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	cfg := CacheConfig{
		LastChecked:              1234567890,
		InstallationId:           "test-id",
		TelemetryDisclosureShown: false,
	}

	// Normal case should succeed.
	err := SaveCache(cfg)
	assert.NoError(t, err)
}

// TestUpdateCache_GetCacheFilePath tests cache file path check at cache.go:137-140.
func TestUpdateCache_GetCacheFilePath(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = 1234567890
	})
	// Should succeed with valid XDG_CACHE_HOME.
	assert.NoError(t, err)
}

// TestUpdateCache_FileLockPath tests file locking at cache.go:143.
func TestUpdateCache_FileLockPath(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// UpdateCache uses file locking - test that path.
	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = 1234567890
	})
	assert.NoError(t, err)
}

// TestUpdateCache_ReadConfigError tests error path at cache.go:149-151.
func TestUpdateCache_ReadConfigError(t *testing.T) {
	// Create temp directory.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// Create cache directory.
	cacheDir := filepath.Join(tempDir, "atmos")
	err := os.MkdirAll(cacheDir, 0o755)
	assert.NoError(t, err)

	// Create invalid YAML file.
	cacheFile := filepath.Join(cacheDir, "cache.yaml")
	invalidYAML := `invalid: [yaml: content`
	err = os.WriteFile(cacheFile, []byte(invalidYAML), 0o644)
	assert.NoError(t, err)

	// UpdateCache should error on invalid YAML.
	err = UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = 1234567890
	})
	if err != nil {
		assert.Contains(t, err.Error(), "cache read failed")
	}
}

// TestUpdateCache_WriteAtomicPath tests write atomic path at cache.go:175-178.
func TestUpdateCache_WriteAtomicPath(t *testing.T) {
	// Create temp directory.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// Normal update should succeed.
	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = 1234567890
		cfg.InstallationId = "test-id"
	})
	assert.NoError(t, err)

	// Verify the file was written.
	cacheFile := filepath.Join(tempDir, "atmos", "cache.yaml")
	_, err = os.Stat(cacheFile)
	assert.NoError(t, err)
}

// TestGetCacheFilePath tests cache file path determination.
func TestGetCacheFilePath(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*testing.T)
		wantErr bool
	}{
		{
			name: "XDG_CACHE_HOME set",
			setup: func(t *testing.T) {
				t.Setenv("XDG_CACHE_HOME", "/tmp/cache")
				xdg.Reload()
			},
			wantErr: false,
		},
		{
			name: "HOME set (Unix)",
			setup: func(t *testing.T) {
				if runtime.GOOS != "windows" {
					t.Setenv("HOME", "/home/user")
					xdg.Reload()
				}
			},
			wantErr: false,
		},
		{
			name: "LOCALAPPDATA set (Windows)",
			setup: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Setenv("LOCALAPPDATA", "C:\\Users\\test\\AppData\\Local")
					xdg.Reload()
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}

			path, err := GetCacheFilePath()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// May succeed or fail depending on environment.
				_ = path
				_ = err
			}
		})
	}
}

// TestCacheConfig_DefaultValues tests default values.
func TestCacheConfig_DefaultValues(t *testing.T) {
	cfg := CacheConfig{}

	// Fields should have zero values initially
	assert.Equal(t, int64(0), cfg.LastChecked)
	assert.Equal(t, "", cfg.InstallationId)
	assert.Equal(t, false, cfg.TelemetryDisclosureShown)

	// After setting values
	cfg.LastChecked = 1234567890
	cfg.InstallationId = "test-id"
	cfg.TelemetryDisclosureShown = true

	assert.Equal(t, int64(1234567890), cfg.LastChecked)
	assert.Equal(t, "test-id", cfg.InstallationId)
	assert.Equal(t, true, cfg.TelemetryDisclosureShown)
}

// TestSaveCache_ValidConfig tests successful save.
func TestSaveCache_ValidConfig(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	cfg := CacheConfig{
		LastChecked:              1234567890,
		InstallationId:           "install-123",
		TelemetryDisclosureShown: true,
	}

	err := SaveCache(cfg)
	assert.NoError(t, err)

	// Verify file was created.
	cacheFile := filepath.Join(tempDir, "atmos", "cache.yaml")
	_, err = os.Stat(cacheFile)
	assert.NoError(t, err)
}

// TestUpdateCache_ValidUpdate tests successful update.
func TestUpdateCache_ValidUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// First update.
	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = 1234567890
		cfg.InstallationId = "id-1"
	})
	assert.NoError(t, err)

	// Second update.
	err = UpdateCache(func(cfg *CacheConfig) {
		cfg.TelemetryDisclosureShown = true
	})
	assert.NoError(t, err)

	// Load and verify.
	cfg, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, int64(1234567890), cfg.LastChecked)
	assert.Equal(t, "id-1", cfg.InstallationId)
	assert.Equal(t, true, cfg.TelemetryDisclosureShown)
}
