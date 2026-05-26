package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstallCmd_Registration(t *testing.T) {
	assert.Equal(t, "install [server-name...]", installCmd.Use)
	assert.NotEmpty(t, installCmd.Short)
	assert.NotEmpty(t, installCmd.Long)
	assert.NotNil(t, installCmd.RunE)

	for _, name := range []string{"client", "all-clients", "scope", "global", "yes", "dry-run", "force", "gitignore"} {
		assert.NotNil(t, installCmd.Flags().Lookup(name), "expected %s flag", name)
	}
}

func TestResolveInstallScope(t *testing.T) {
	t.Run("global alias selects user scope", func(t *testing.T) {
		cmd := newMCPInstallTestCmd(t)
		v := viper.New()
		v.Set("scope", mcpinstall.ScopeProject)
		v.Set("global", true)

		assert.Equal(t, mcpinstall.ScopeUser, resolveInstallScope(cmd, v))
	})

	t.Run("explicit scope beats global from env", func(t *testing.T) {
		cmd := newMCPInstallTestCmd(t)
		require.NoError(t, cmd.Flags().Set("scope", mcpinstall.ScopeProject))
		v := viper.New()
		v.Set("scope", mcpinstall.ScopeProject)
		v.Set("global", true)

		assert.Equal(t, mcpinstall.ScopeProject, resolveInstallScope(cmd, v))
	})
}

func TestMCPConflictHandlerYesSkips(t *testing.T) {
	overwrite, err := mcpConflictHandler(true)(mcpinstall.Target{}, "atmos-pro")
	require.NoError(t, err)
	assert.False(t, overwrite)
}

func TestInstallServersYesSkipsExistingServer(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"test":{"command":"old"}}}`), 0o600))

	err := installServers(&schema.AtmosConfiguration{BasePath: base}, map[string]schema.MCPServerConfig{
		"test": {Command: "new"},
	}, installCommandOptions{
		clients: []string{mcpinstall.ClientClaudeCode},
		scope:   mcpinstall.ScopeProject,
		yes:     true,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"old"`)
	assert.NotContains(t, string(data), `"new"`)
}

func newMCPInstallTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)
	return cmd
}
