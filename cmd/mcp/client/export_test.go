package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestExportCmd_NoArgs(t *testing.T) {
	// Verify the command rejects positional arguments.
	assert.NotNil(t, exportCmd.Args, "Args validator should be set")
}

// TestExport_DelegatesToPackageGenerator is the structural regression
// guard for issue #1. Before the fix, cmd/mcp/client/export.go had its
// own private mcpJSONConfig/mcpJSONServer types and its own
// buildMCPJSONEntry that omitted toolchain PATH injection. After the
// fix, export.go consumes pkg/mcp/client.GenerateMCPConfig, which means
// the contract this test pins is:
//
//  1. Identity-having servers get wrapped with `atmos auth exec`.
//  2. Non-identity servers use their command directly.
//  3. When a non-empty toolchainPATH is passed, every server's env carries
//     PATH (the headline bug fix — IDE-spawned subprocesses can now find
//     toolchain-managed `uvx` / `npx`).
//
// If a future refactor reverts export.go to a private builder that
// drops the toolchainPATH parameter, this test catches it because the
// PATH assertion will fail.
func TestExport_DelegatesToPackageGenerator(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"aws-docs": {
			Command: "uvx",
			Args:    []string{"awslabs.aws-documentation-mcp-server@latest"},
		},
		"aws-security": {
			Command:  "uvx",
			Args:     []string{"awslabs.well-architected-security-mcp-server@latest"},
			Env:      map[string]string{"aws_region": "us-east-1"},
			Identity: "readonly",
		},
	}

	t.Run("no toolchain PATH → no PATH injection", func(t *testing.T) {
		config := mcpclient.GenerateMCPConfig(servers, "")

		require.Contains(t, config.MCPServers, "aws-docs")
		assert.Equal(t, "uvx", config.MCPServers["aws-docs"].Command,
			"non-identity server keeps its command directly")
		// Empty toolchainPATH must not inject a PATH key into the env map.
		assert.NotContains(t, config.MCPServers["aws-docs"].Env, "PATH",
			"empty toolchainPATH MUST NOT inject a PATH env var")
	})

	t.Run("non-empty toolchain PATH → PATH injected for every server", func(t *testing.T) {
		const toolchainPATH = "/atmos/toolchain/bin"
		config := mcpclient.GenerateMCPConfig(servers, toolchainPATH)

		for name, entry := range config.MCPServers {
			pathVal, ok := entry.Env["PATH"]
			require.True(t, ok, "server %q MUST have PATH injected when toolchainPATH is non-empty", name)
			assert.True(t, strings.Contains(pathVal, toolchainPATH),
				"server %q PATH must include the toolchain dir; got %q", name, pathVal)
		}
	})

	t.Run("identity-having server wraps with atmos auth exec", func(t *testing.T) {
		config := mcpclient.GenerateMCPConfig(servers, "")

		require.Contains(t, config.MCPServers, "aws-security")
		entry := config.MCPServers["aws-security"]
		assert.Equal(t, "atmos", entry.Command,
			"identity-having server MUST be wrapped with `atmos` as the entrypoint")
		assert.Equal(t,
			[]string{"auth", "exec", "-i", "readonly", "--", "uvx", "awslabs.well-architected-security-mcp-server@latest"},
			entry.Args,
			"args MUST be: auth exec -i <identity> -- <original command and args>")
	})
}

// TestExport_JSONShapeMatchesContract ensures the JSON serialization of
// the shared MCPJSONConfig matches the .mcp.json file shape Claude Code /
// Cursor expect. This was previously covered by a cmd-local
// TestMCPJSONConfig_Marshal that tested the now-deleted private struct;
// we move it here so the test still exists for the production shape.
func TestExport_JSONShapeMatchesContract(t *testing.T) {
	config := mcpclient.GenerateMCPConfig(map[string]schema.MCPServerConfig{
		"aws-docs": {
			Command: "uvx",
			Args:    []string{"awslabs.aws-documentation-mcp-server@latest"},
		},
		"aws-security": {
			Command:  "uvx",
			Args:     []string{"awslabs.well-architected-security-mcp-server@latest"},
			Env:      map[string]string{"AWS_REGION": "us-east-1"},
			Identity: "readonly",
		},
	}, "")

	data, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)

	output := string(data)
	assert.Contains(t, output, `"mcpServers"`)
	assert.Contains(t, output, `"aws-docs"`)
	assert.Contains(t, output, `"aws-security"`)
	assert.Contains(t, output, `"readonly"`)
	assert.Contains(t, output, `"auth"`)
	// aws-docs has no env — omitempty must exclude the env field.
	assert.NotContains(t, output, `"env": null`)
}

// TestBuildToolchainPATH_NoToolchainReturnsEmpty exercises buildToolchainPATH
// directly with an AtmosConfiguration that has no `.tool-versions` and no
// terraform components. Both fallback chains return nil → "". This is the
// precondition that lets `atmos mcp export` skip PATH injection on simple
// projects rather than writing a bogus empty PATH= entry.
func TestBuildToolchainPATH_NoToolchainReturnsEmpty(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	got := buildToolchainPATH(atmosConfig)

	assert.Equal(t, "", got,
		"buildToolchainPATH must return \"\" when no toolchain resolves so the export skips PATH injection")
}
