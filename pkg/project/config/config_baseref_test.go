package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadUserConfigWithBaseRef(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "config-baseref-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test data
	templateID := "simple"
	baseRef := "main"
	values := map[string]interface{}{
		"project_name": "test-project",
		"aws_region":   "us-east-1",
	}

	// Save config with base ref
	err = SaveUserConfigWithBaseRef(tmpDir, templateID, baseRef, values)
	require.NoError(t, err)

	// Verify file was created
	configPath := filepath.Join(tmpDir, ScaffoldConfigDir, ScaffoldConfigFileName)
	assert.FileExists(t, configPath)

	// Load config back
	userConfig, err := LoadUserConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, userConfig)

	// Verify values
	assert.Equal(t, templateID, userConfig.TemplateID)
	assert.Equal(t, baseRef, userConfig.BaseRef)
	assert.Equal(t, "test-project", userConfig.Values["project_name"])
	assert.Equal(t, "us-east-1", userConfig.Values["aws_region"])
}

func TestSaveUserConfigWithBaseRef_EmptyBaseRef(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "config-baseref-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test data with empty base ref
	templateID := "simple"
	baseRef := ""
	values := map[string]interface{}{
		"project_name": "test-project",
	}

	// Save config with empty base ref
	err = SaveUserConfigWithBaseRef(tmpDir, templateID, baseRef, values)
	require.NoError(t, err)

	// Load config back
	userConfig, err := LoadUserConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, userConfig)

	// Verify values - empty base ref should not be present in YAML
	assert.Equal(t, templateID, userConfig.TemplateID)
	assert.Empty(t, userConfig.BaseRef) // Should be empty
	assert.Equal(t, "test-project", userConfig.Values["project_name"])
}

func TestLoadUserConfig_NonexistentFile(t *testing.T) {
	// Create temp directory without config file
	tmpDir, err := os.MkdirTemp("", "config-baseref-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Load config from directory without file
	userConfig, err := LoadUserConfig(tmpDir)
	require.NoError(t, err)
	assert.Nil(t, userConfig) // Should return nil when file doesn't exist
}

func TestSaveUserConfig_BackwardsCompatibility(t *testing.T) {
	// Test that SaveUserConfig (without base ref) still works
	tmpDir, err := os.MkdirTemp("", "config-baseref-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	templateID := "simple"
	values := map[string]interface{}{
		"project_name": "test-project",
	}

	// Save config without base ref (backwards compatibility)
	err = SaveUserConfig(tmpDir, templateID, values)
	require.NoError(t, err)

	// Load config back
	userConfig, err := LoadUserConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, userConfig)

	// Verify values - base ref should be empty
	assert.Equal(t, templateID, userConfig.TemplateID)
	assert.Empty(t, userConfig.BaseRef)
	assert.Equal(t, "test-project", userConfig.Values["project_name"])
}
