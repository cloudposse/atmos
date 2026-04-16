package client

import (
	"encoding/json"
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

func TestMCPJSONConfig_Marshal(t *testing.T) {
	config := mcpJSONConfig{
		MCPServers: map[string]mcpJSONServer{
			"aws-docs": {
				Command: "uvx",
				Args:    []string{"awslabs.aws-documentation-mcp-server@latest"},
			},
			"aws-security": {
				Command: "atmos",
				Args:    []string{"auth", "exec", "-i", "readonly", "--", "uvx", "awslabs.well-architected-security-mcp-server@latest"},
				Env:     map[string]string{"AWS_REGION": "us-east-1"},
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)

	output := string(data)
	assert.Contains(t, output, `"mcpServers"`)
	assert.Contains(t, output, `"aws-docs"`)
	assert.Contains(t, output, `"aws-security"`)
	assert.Contains(t, output, `"readonly"`)
	assert.Contains(t, output, `"auth"`)
	// aws-docs has no env — omitempty should exclude it.
	assert.NotContains(t, output, `"env": null`)
}

func TestExportCmd_NoArgs(t *testing.T) {
	// Verify the command rejects positional arguments.
	assert.NotNil(t, exportCmd.Args, "Args validator should be set")
}
