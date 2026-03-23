package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAddServerToConfig_NewSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create a minimal atmos.yaml without mcp section.
	require.NoError(t, os.WriteFile(configFile, []byte("base_path: .\n"), 0o644))

	server := map[string]any{
		"command":     "uvx",
		"description": "Test server",
	}

	err := addServerToConfig(configFile, "test-server", server)
	require.NoError(t, err)

	// Read back and verify.
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	mcpSection, ok := config["mcp"].(map[string]any)
	require.True(t, ok, "mcp section should exist")

	servers, ok := mcpSection["servers"].(map[string]any)
	require.True(t, ok, "servers section should exist")

	testServer, ok := servers["test-server"].(map[string]any)
	require.True(t, ok, "test-server should exist")
	assert.Equal(t, "uvx", testServer["command"])
	assert.Equal(t, "Test server", testServer["description"])
}

func TestAddServerToConfig_ExistingSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create atmos.yaml with existing mcp.servers.
	initial := `base_path: .
mcp:
  enabled: true
  servers:
    existing-server:
      command: echo
`
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o644))

	server := map[string]any{
		"command": "uvx",
	}

	err := addServerToConfig(configFile, "new-server", server)
	require.NoError(t, err)

	// Verify both servers exist.
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	servers := config["mcp"].(map[string]any)["servers"].(map[string]any)
	assert.Contains(t, servers, "existing-server")
	assert.Contains(t, servers, "new-server")
}

func TestRemoveServerFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	initial := `mcp:
  servers:
    server-a:
      command: echo
    server-b:
      command: cat
`
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o644))

	err := removeServerFromConfig(configFile, "server-a")
	require.NoError(t, err)

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))

	servers := config["mcp"].(map[string]any)["servers"].(map[string]any)
	assert.NotContains(t, servers, "server-a")
	assert.Contains(t, servers, "server-b")
}

func TestRemoveServerFromConfig_NoMCPSection(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	require.NoError(t, os.WriteFile(configFile, []byte("base_path: .\n"), 0o644))

	// Should not error when mcp section doesn't exist.
	err := removeServerFromConfig(configFile, "nonexistent")
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
