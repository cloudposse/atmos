package docker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func setupTestDockerConfigDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	// Set DOCKER_CONFIG to isolate tests from user's actual Docker config.
	t.Setenv("DOCKER_CONFIG", tmpDir)
	return tmpDir
}

func TestNewConfigManager(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.Equal(t, tmpDir, manager.GetConfigDir())
	assert.Contains(t, manager.GetConfigPath(), "config.json")
}

func TestConfigManager_WriteAuth(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	registry := "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	username := "AWS"
	password := "test-token"

	err = manager.WriteAuth(registry, username, password)
	require.NoError(t, err)

	// Verify the config file was created.
	data, err := os.ReadFile(manager.GetConfigPath())
	require.NoError(t, err)

	var config dockerConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify the auth entry exists.
	authEntry, exists := config.Auths[registry]
	assert.True(t, exists, "registry should exist in auths")

	// Verify the credentials are base64 encoded.
	expectedAuth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	assert.Equal(t, expectedAuth, authEntry.Auth)
}

func TestConfigManager_WriteAuth_MultipleRegistries(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	registry1 := "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	registry2 := "123456789012.dkr.ecr.us-west-2.amazonaws.com"

	err = manager.WriteAuth(registry1, "AWS", "token1")
	require.NoError(t, err)

	err = manager.WriteAuth(registry2, "AWS", "token2")
	require.NoError(t, err)

	// Read the config file.
	data, err := os.ReadFile(manager.GetConfigPath())
	require.NoError(t, err)

	var config dockerConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify both registries exist.
	assert.Len(t, config.Auths, 2)
	assert.Contains(t, config.Auths, registry1)
	assert.Contains(t, config.Auths, registry2)
}

func TestConfigManager_WriteAuth_UpdateExisting(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	registry := "123456789012.dkr.ecr.us-east-1.amazonaws.com"

	// Write initial auth.
	err = manager.WriteAuth(registry, "AWS", "old-token")
	require.NoError(t, err)

	// Update with new token.
	err = manager.WriteAuth(registry, "AWS", "new-token")
	require.NoError(t, err)

	// Read the config file.
	data, err := os.ReadFile(manager.GetConfigPath())
	require.NoError(t, err)

	var config dockerConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify the token was updated.
	authEntry := config.Auths[registry]
	expectedAuth := base64.StdEncoding.EncodeToString([]byte("AWS:new-token"))
	assert.Equal(t, expectedAuth, authEntry.Auth)
}

func TestConfigManager_RemoveAuth(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	registry := "123456789012.dkr.ecr.us-east-1.amazonaws.com"

	// Write auth first.
	err = manager.WriteAuth(registry, "AWS", "token")
	require.NoError(t, err)

	// Remove auth.
	err = manager.RemoveAuth(registry)
	require.NoError(t, err)

	// Read the config file.
	data, err := os.ReadFile(manager.GetConfigPath())
	require.NoError(t, err)

	var config dockerConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify the registry was removed.
	_, exists := config.Auths[registry]
	assert.False(t, exists, "registry should not exist after removal")
}

func TestConfigManager_RemoveAuth_NonExistent(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Removing a non-existent registry should not error.
	err = manager.RemoveAuth("non-existent-registry")
	assert.NoError(t, err)
}

func TestConfigManager_GetConfigDir(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	assert.Equal(t, tmpDir, manager.GetConfigDir())
}

func TestConfigManager_GetAuthenticatedRegistries(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Initially empty.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Empty(t, registries)

	// Add some registries.
	err = manager.WriteAuth("registry1.example.com", "user", "pass")
	require.NoError(t, err)
	err = manager.WriteAuth("registry2.example.com", "user", "pass")
	require.NoError(t, err)

	registries, err = manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Len(t, registries, 2)
	assert.Contains(t, registries, "registry1.example.com")
	assert.Contains(t, registries, "registry2.example.com")
}

func TestConfigManager_PreservesExistingConfig(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	// First create a manager to get the actual config path.
	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Create an existing config with custom fields.
	existingConfig := dockerConfig{
		Auths: map[string]authEntry{
			"existing-registry.com": {Auth: "existing-auth"},
		},
	}
	data, err := json.MarshalIndent(existingConfig, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(manager.GetConfigPath(), data, 0o600)
	require.NoError(t, err)

	// Add a new registry.
	err = manager.WriteAuth("new-registry.com", "user", "pass")
	require.NoError(t, err)

	// Verify both registries exist.
	data, err = os.ReadFile(manager.GetConfigPath())
	require.NoError(t, err)

	var config dockerConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	assert.Len(t, config.Auths, 2)
	assert.Contains(t, config.Auths, "existing-registry.com")
	assert.Contains(t, config.Auths, "new-registry.com")
}

func TestConfigManager_GetConfigPath(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	expectedPath := filepath.Join(tmpDir, "config.json")
	assert.Equal(t, expectedPath, manager.GetConfigPath())
}

func TestConfigManager_WithConfigLock_WrapsErrCacheLocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows uses a best-effort no-op FileLock (see pkg/cache/filelock_windows.go)
		// that never returns ErrCacheLocked, so this branch cannot be exercised there.
		t.Skip("Windows FileLock is a no-op and never reports a lock timeout")
	}

	tempDir := t.TempDir()

	// Point configPath at a parent directory that does not exist. gofrs/flock
	// (as vendored here) opens its lock file with O_RDONLY|O_CREATE on most
	// platforms, so a pre-created directory at the lock path is silently
	// flock()-able and does NOT produce an error (unlike older flock
	// versions that opened O_RDWR). A missing parent directory, however,
	// fails the underlying open() immediately and deterministically on every
	// platform, giving withConfigLock a hard, non-blocking error without
	// needing to hold a real contending lock.
	configPath := filepath.Join(tempDir, "nonexistent-subdir", "config.json")

	manager := &ConfigManager{configPath: configPath}

	executed := false
	err := manager.withConfigLock(func() error {
		executed = true
		return nil
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigWrite))
	assert.False(t, executed, "fn must not run when the lock cannot be acquired")
}

// TestNewConfigManager_MkdirAllFailure verifies that a MkdirAll failure (the
// config directory already exists as a regular file, not a directory) is
// surfaced as ErrDockerConfigWrite instead of panicking or being silently
// ignored.
func TestNewConfigManager_MkdirAllFailure(t *testing.T) {
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "docker-config")
	require.NoError(t, os.WriteFile(blockingFile, []byte("not a directory"), 0o600))
	t.Setenv("DOCKER_CONFIG", blockingFile)

	manager, err := NewConfigManager()
	require.Error(t, err)
	assert.Nil(t, manager)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigWrite))
}

// TestConfigManager_LoadConfig_ReadFailure verifies the non-ENOENT
// os.ReadFile failure branch (the config path is a directory, not a missing
// file).
func TestConfigManager_LoadConfig_ReadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.MkdirAll(configPath, 0o700)) // Directory in place of the file.

	manager := &ConfigManager{configDir: tmpDir, configPath: configPath}
	_, err := manager.loadConfig()
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigRead))
}

// TestConfigManager_SaveConfig_WriteFailure verifies that an os.WriteFile
// failure (a read-only config directory) is surfaced as ErrDockerConfigWrite.
// The saveConfig method has no locking of its own (its callers wrap it in
// withConfigLock), so it's exercised directly here.
func TestConfigManager_SaveConfig_WriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission bits are not enforced the same way on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.Chmod(tmpDir, 0o500)) // Read-only: blocks creating config.json.
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o700) })

	manager := &ConfigManager{configDir: tmpDir, configPath: configPath}
	err := manager.saveConfig(&dockerConfig{Auths: map[string]authEntry{"registry": {Auth: "dGVzdA=="}}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigWrite))
}

// TestConfigManager_WriteAuth_LoadConfigFailureAfterLock verifies that a
// loadConfig failure occurring *inside* the lock closure (the config path is
// a directory, so the sibling ".lock" file still locks successfully but the
// read itself fails) is returned by WriteAuth as-is, and is not mistaken for
// a lock-acquisition timeout.
func TestConfigManager_WriteAuth_LoadConfigFailureAfterLock(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.MkdirAll(configPath, 0o700)) // Directory in place of the file.

	manager := &ConfigManager{configDir: tmpDir, configPath: configPath}
	err := manager.WriteAuth("registry.example.com", "user", "pass")

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigRead))
	assert.False(t, errors.Is(err, errUtils.ErrDockerConfigWrite), "a read failure must not be mistaken for a lock-acquisition (write) failure")
}

// TestConfigManager_RemoveAuth_LoadConfigFailureAfterLock mirrors
// TestConfigManager_WriteAuth_LoadConfigFailureAfterLock for RemoveAuth's own
// loadConfig call site inside its lock closure.
func TestConfigManager_RemoveAuth_LoadConfigFailureAfterLock(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.MkdirAll(configPath, 0o700)) // Directory in place of the file.

	manager := &ConfigManager{configDir: tmpDir, configPath: configPath}
	err := manager.RemoveAuth("registry.example.com")

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigRead))
}
