package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportCmd_Registration(t *testing.T) {
	assert.Equal(t, "export", exportCmd.Use)
	assert.NotEmpty(t, exportCmd.Short)
	assert.NotEmpty(t, exportCmd.Long)
	assert.NotNil(t, exportCmd.RunE)
}

func TestExportCmd_OutputFlag(t *testing.T) {
	flag := exportCmd.Flags().Lookup("output")
	require.NotNil(t, flag, "output flag should be registered")
	assert.Equal(t, "o", flag.Shorthand)
	assert.Equal(t, ".mcp.json", flag.DefValue)
}

func TestExportLongMarkdown(t *testing.T) {
	assert.Contains(t, exportLongMarkdown, ".mcp.json")
	assert.Contains(t, exportLongMarkdown, "atmos mcp export")
}

func TestConfigFilePermissions(t *testing.T) {
	// Verify the file permissions constant is owner-only (0600).
	assert.Equal(t, 0o600, int(configFilePermissions))
}

func TestMCPJSONConfig_Structure(t *testing.T) {
	// Verify the mcpJSONConfig and mcpJSONServer types work correctly.
	config := mcpJSONConfig{
		MCPServers: map[string]mcpJSONServer{
			"test": {
				Command: "echo",
				Args:    []string{"hello"},
				Env:     map[string]string{"KEY": "val"},
			},
		},
	}
	assert.Len(t, config.MCPServers, 1)
	assert.Equal(t, "echo", config.MCPServers["test"].Command)
	assert.Equal(t, []string{"hello"}, config.MCPServers["test"].Args)
	assert.Equal(t, "val", config.MCPServers["test"].Env["KEY"])
}
