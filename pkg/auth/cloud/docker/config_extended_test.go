package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDockerConfigDir_WithDOCKER_CONFIG(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)

	result := getDockerConfigDir()
	assert.Equal(t, tmpDir, result)
}

func TestGetDockerConfigDir_WithoutDOCKER_CONFIG(t *testing.T) {
	// Unset DOCKER_CONFIG to test default behavior.
	t.Setenv("DOCKER_CONFIG", "")

	result := getDockerConfigDir()
	// Should return ~/.docker path.
	assert.Contains(t, result, ".docker")
}

func TestConfigManager_LoadConfig_EmptyFile(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Create an empty config file.
	err = os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{}"), 0o600)
	require.NoError(t, err)

	// Should handle empty config gracefully.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Empty(t, registries)
}

func TestConfigManager_LoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Create an invalid JSON file.
	err = os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{invalid json}"), 0o600)
	require.NoError(t, err)

	// Should return error for invalid JSON.
	_, err = manager.GetAuthenticatedRegistries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestConfigManager_LoadConfig_NullAuths(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Create a config with null auths.
	configData := `{"auths": null}`
	err = os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(configData), 0o600)
	require.NoError(t, err)

	// Should handle null auths gracefully.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Empty(t, registries)
}

func TestConfigManager_RemoveAuth_MultipleRegistries(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Add multiple registries.
	err = manager.WriteAuth("registry1.example.com", "user1", "pass1")
	require.NoError(t, err)
	err = manager.WriteAuth("registry2.example.com", "user2", "pass2")
	require.NoError(t, err)
	err = manager.WriteAuth("registry3.example.com", "user3", "pass3")
	require.NoError(t, err)

	// Remove multiple at once.
	err = manager.RemoveAuth("registry1.example.com", "registry3.example.com")
	require.NoError(t, err)

	// Verify only registry2 remains.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Len(t, registries, 1)
	assert.Contains(t, registries, "registry2.example.com")
}

func TestConfigManager_WriteAuth_WithExistingOtherFields(t *testing.T) {
	tmpDir := setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Create an existing config with other fields (not just auths).
	existingConfig := map[string]interface{}{
		"auths": map[string]interface{}{},
		"credHelpers": map[string]string{
			"gcr.io": "gcloud",
		},
		"experimental": "enabled",
	}
	data, err := json.MarshalIndent(existingConfig, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0o600)
	require.NoError(t, err)

	// Add a new registry.
	err = manager.WriteAuth("new-registry.example.com", "user", "pass")
	require.NoError(t, err)

	// Verify the auth was added.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Contains(t, registries, "new-registry.example.com")
}

func TestConfigManager_ConcurrentWrites(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Perform concurrent writes (testing mutex).
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			registry := "registry" + string(rune('0'+idx)) + ".example.com"
			_ = manager.WriteAuth(registry, "user", "pass")
			done <- true
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have all 10 registries.
	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Len(t, registries, 10)
}

func TestConfigManager_GetConfigPath_Absolute(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	path := manager.GetConfigPath()
	assert.True(t, filepath.IsAbs(path), "config path should be absolute")
}

func TestConfigManager_WriteAuth_EmptyRegistry(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Writing with empty registry should still work (no validation at this level).
	err = manager.WriteAuth("", "user", "pass")
	require.NoError(t, err)

	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Contains(t, registries, "")
}

func TestConfigManager_WriteAuth_EmptyCredentials(t *testing.T) {
	_ = setupTestDockerConfigDir(t)

	manager, err := NewConfigManager()
	require.NoError(t, err)

	// Writing with empty username/password should work.
	err = manager.WriteAuth("registry.example.com", "", "")
	require.NoError(t, err)

	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Contains(t, registries, "registry.example.com")
}
