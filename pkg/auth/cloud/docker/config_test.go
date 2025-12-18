package docker

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	path := manager.GetConfigPath()
	assert.Equal(t, tmpDir+"/config.json", path)
}
