package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// newExportTestCmd builds a cobra.Command with the same flag surface that
// `atmos mcp export` registers. Tests inject it into executeMCPExport so
// the function-under-test sees a realistic command instance with an
// `--output` flag.
func newExportTestCmd(t *testing.T, outputPath string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "export"}
	cmd.Flags().StringP("output", "o", outputPath, "Output file path")
	return cmd
}

// writeMinimalAtmosYAMLForExport writes an atmos.yaml that
// cfg.InitCliConfig will accept (base_path, optional mcp.servers) into a
// temp directory and chdir's into it so the function-under-test loads it
// via the standard discovery path.
//
// The `servers` arg is the YAML body for the `mcp.servers:` block,
// indented to the schema's expected level. Pass "" to omit the block
// entirely (exercises the "no servers configured" branch).
func writeMinimalAtmosYAMLForExport(t *testing.T, servers string) string {
	t.Helper()
	tempDir := t.TempDir()
	body := "base_path: \".\"\n"
	if servers != "" {
		body += "mcp:\n  servers:\n" + servers
	}
	require.NoError(t,
		os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(body), 0o644))
	t.Chdir(tempDir)
	return tempDir
}

// TestExecuteMCPExport_NoServersEarlyReturn covers the "no MCP servers
// configured" branch of executeMCPExport — the function should emit an
// info notice and return nil without creating an output file. Before the
// issue-#1 refactor, both paths (no-servers and with-servers) used
// duplicated logic; locking this branch in prevents an accidental
// regression where the early return is removed when someone simplifies
// the function.
func TestExecuteMCPExport_NoServersEarlyReturn(t *testing.T) {
	tempDir := writeMinimalAtmosYAMLForExport(t, "")

	outputPath := filepath.Join(tempDir, "out.mcp.json")
	cmd := newExportTestCmd(t, outputPath)

	err := executeMCPExport(cmd, nil)
	require.NoError(t, err,
		"executeMCPExport must return nil when no servers are configured (info notice only)")

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr),
		"executeMCPExport must NOT create an output file when no servers are configured; got stat err: %v", statErr)
}

// TestExecuteMCPExport_HappyPath is the end-to-end regression guard for
// issue #1. With one stdio MCP server defined in atmos.yaml,
// executeMCPExport must:
//
//  1. Generate the output file at the path given by --output.
//  2. Write valid JSON in the .mcp.json shape (top-level "mcpServers").
//  3. Carry the server's command/args through unchanged.
//  4. Apply 0600 file permissions (issue #4-style "secrets-adjacent" file).
//
// The "atmos" server is used because it doesn't require uvx or any
// subprocess — `atmos mcp export` only writes config, it doesn't spawn
// the servers.
func TestExecuteMCPExport_HappyPath(t *testing.T) {
	tempDir := writeMinimalAtmosYAMLForExport(t, `    atmos:
      command: atmos
      args: ["mcp", "start"]
      description: "Atmos AI tools"
`)

	outputPath := filepath.Join(tempDir, "exported.mcp.json")
	cmd := newExportTestCmd(t, outputPath)

	err := executeMCPExport(cmd, nil)
	require.NoError(t, err)

	// Output file must exist.
	info, err := os.Stat(outputPath)
	require.NoError(t, err, "executeMCPExport must create the output file")

	// File permissions: 0600 (issue #1's package-builder delegation
	// preserves the permission semantics from the cmd-local code that
	// was removed).
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"exported config must be 0600 — it's adjacent to credential context (identity wrapping)")
	}

	// Content: valid JSON with the expected shape.
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var parsed mcpclient.MCPJSONConfig
	require.NoError(t, json.Unmarshal(data, &parsed),
		"exported file MUST be valid JSON parsing into MCPJSONConfig")

	require.Contains(t, parsed.MCPServers, "atmos",
		"exported config must contain the `atmos` server entry from atmos.yaml")
	entry := parsed.MCPServers["atmos"]
	assert.Equal(t, "atmos", entry.Command)
	assert.Equal(t, []string{"mcp", "start"}, entry.Args)
}

// TestExecuteMCPExport_OverwritesExistingFile guards the os.Chmod-after-write
// branch in executeMCPExport. When the output file already exists,
// os.WriteFile preserves the OLD permissions — so the function explicitly
// runs os.Chmod afterwards to enforce 0600. This test creates a 0644 file
// up front and asserts executeMCPExport tightens it to 0600.
func TestExecuteMCPExport_OverwritesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode bits are not meaningful on Windows")
	}

	tempDir := writeMinimalAtmosYAMLForExport(t, `    atmos:
      command: atmos
      args: ["mcp", "start"]
`)

	outputPath := filepath.Join(tempDir, "preexisting.mcp.json")
	// Pre-create with loose permissions; executeMCPExport must tighten.
	require.NoError(t, os.WriteFile(outputPath, []byte("stale"), 0o644))

	cmd := newExportTestCmd(t, outputPath)
	require.NoError(t, executeMCPExport(cmd, nil))

	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"executeMCPExport must run os.Chmod after WriteFile so EXISTING output files are tightened to 0600")

	// And the content must be the new JSON, not the stale string.
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEqual(t, "stale", string(data))
	assert.True(t, strings.HasPrefix(string(data), "{"),
		"expected JSON content; got %q", string(data))
}

// TestExecuteMCPExport_WriteFailure_NonExistentDir verifies the
// os.WriteFile error path is reached and wrapped with the right
// sentinel. Without this, an export pointing at a missing directory
// would surface a bare os.PathError that callers can't programmatically
// match against errUtils.ErrMCPConfigWriteFailed.
func TestExecuteMCPExport_WriteFailure_NonExistentDir(t *testing.T) {
	writeMinimalAtmosYAMLForExport(t, `    atmos:
      command: atmos
      args: ["mcp", "start"]
`)

	// /does-not-exist/.../out.mcp.json — WriteFile fails because the
	// parent directory doesn't exist.
	outputPath := filepath.Join(t.TempDir(), "does", "not", "exist", "out.mcp.json")
	cmd := newExportTestCmd(t, outputPath)

	err := executeMCPExport(cmd, nil)
	require.Error(t, err, "writing to a non-existent directory must return an error")
	assert.Contains(t, err.Error(), "does/not/exist",
		"error must include the offending path for actionable diagnostics; got: %v", err)
}
