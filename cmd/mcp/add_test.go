package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAddIntegrationToConfig_NewSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create a minimal atmos.yaml without mcp section.
	require.NoError(t, os.WriteFile(configFile, []byte("base_path: .\n"), 0o644))

	integration := map[string]any{
		"command":     "uvx",
		"description": "Test server",
	}

	err := addIntegrationToConfig(configFile, "test-server", integration)
	require.NoError(t, err)

	// Read back and verify.
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	mcpSection, ok := config["mcp"].(map[string]any)
	require.True(t, ok, "mcp section should exist")

	integrations, ok := mcpSection["integrations"].(map[string]any)
	require.True(t, ok, "integrations section should exist")

	server, ok := integrations["test-server"].(map[string]any)
	require.True(t, ok, "test-server should exist")
	assert.Equal(t, "uvx", server["command"])
	assert.Equal(t, "Test server", server["description"])
}

func TestAddIntegrationToConfig_ExistingSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create atmos.yaml with existing mcp.integrations.
	initial := `base_path: .
mcp:
  enabled: true
  integrations:
    existing-server:
      command: echo
`
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o644))

	integration := map[string]any{
		"command": "uvx",
	}

	err := addIntegrationToConfig(configFile, "new-server", integration)
	require.NoError(t, err)

	// Verify both servers exist.
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	integrations := config["mcp"].(map[string]any)["integrations"].(map[string]any)
	assert.Contains(t, integrations, "existing-server")
	assert.Contains(t, integrations, "new-server")
}

func TestRemoveIntegrationFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	initial := `mcp:
  integrations:
    server-a:
      command: echo
    server-b:
      command: cat
`
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o644))

	err := removeIntegrationFromConfig(configFile, "server-a")
	require.NoError(t, err)

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	integrations := config["mcp"].(map[string]any)["integrations"].(map[string]any)
	assert.NotContains(t, integrations, "server-a")
	assert.Contains(t, integrations, "server-b")
}

func TestRemoveIntegrationFromConfig_NoMCPSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte("base_path: .\n"), 0o644))

	// Should not error when mcp section doesn't exist.
	err := removeIntegrationFromConfig(configFile, "nonexistent")
	require.NoError(t, err)
}

func TestFindAtmosYAML_Fallback(t *testing.T) {
	// When no file exists at the config path, defaults to "atmos.yaml".
	result := findAtmosYAML("/nonexistent/path")
	assert.Equal(t, "atmos.yaml", result)
}

func TestFindAtmosYAML_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(""), 0o644))

	result := findAtmosYAML(tmpDir)
	assert.Equal(t, configFile, result)
}
