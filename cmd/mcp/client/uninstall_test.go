package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUninstallCmd_Registration(t *testing.T) {
	assert.Equal(t, "uninstall [server-name...]", uninstallCmd.Use)
	assert.NotEmpty(t, uninstallCmd.Short)
	assert.NotEmpty(t, uninstallCmd.Long)
	assert.NotNil(t, uninstallCmd.RunE)

	for _, name := range []string{"client", "all-clients", "scope", "global", "yes", "dry-run"} {
		assert.NotNil(t, uninstallCmd.Flags().Lookup(name), "expected %s flag", name)
	}
	for _, name := range []string{"force", "gitignore"} {
		assert.Nil(t, uninstallCmd.Flags().Lookup(name), "uninstall has no overwrite/gitignore semantics, unlike install")
	}
}

func TestSortedServerNames(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"zebra": {Command: "a"},
		"alpha": {Command: "b"},
		"mid":   {Command: "c"},
	}
	assert.Equal(t, []string{"alpha", "mid", "zebra"}, sortedServerNames(servers))
	assert.Empty(t, sortedServerNames(nil))
}

func newUninstallTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "uninstall"}
	uninstallParser.RegisterFlags(cmd)
	return cmd
}

func TestExecuteMCPUninstall_DefaultsToAllConfiguredServers(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"),
		[]byte("base_path: \".\"\nmcp:\n  servers:\n    aws-docs:\n      command: uvx\n"), 0o644))
	t.Chdir(tempDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".cursor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".cursor", "mcp.json"),
		[]byte(`{"mcpServers":{"aws-docs":{"command":"uvx"}}}`), 0o600))

	cmd := newUninstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("client", "cursor"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	err := executeMCPUninstall(cmd, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tempDir, ".cursor", "mcp.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "aws-docs")
}

func TestExecuteMCPUninstall_NoServersConfiguredIsNoop(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("base_path: \".\"\n"), 0o644))
	t.Chdir(tempDir)

	cmd := newUninstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	err := executeMCPUninstall(cmd, nil)
	require.NoError(t, err, "no configured servers and no explicit names must be a graceful no-op")
}

func TestExecuteMCPUninstall_NoClientsDetectedIsNoop(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"),
		[]byte("base_path: \".\"\nmcp:\n  servers:\n    aws-docs:\n      command: uvx\n"), 0o644))
	t.Chdir(tempDir)

	cmd := newUninstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	// No --client/--all-clients, and no AI client directories exist in
	// tempDir, so auto-detection finds nothing. This must skip gracefully
	// rather than erroring or falling back to every supported client.
	err := executeMCPUninstall(cmd, nil)
	require.NoError(t, err)
}
