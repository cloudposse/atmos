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

func TestWithUserScopeCwd(t *testing.T) {
	abs, err := filepath.Abs("testdata-project")
	require.NoError(t, err)

	tests := []struct {
		name    string
		servers map[string]schema.MCPServerConfig
		wantCwd map[string]string
	}{
		{
			name: "stdio server without cwd gets project abs path",
			servers: map[string]schema.MCPServerConfig{
				"self": {Command: "atmos", Args: []string{"mcp", "start"}},
			},
			wantCwd: map[string]string{"self": abs},
		},
		{
			name: "stdio server with explicit cwd is left alone",
			servers: map[string]schema.MCPServerConfig{
				"self": {Command: "atmos", Args: []string{"mcp", "start"}, Cwd: "/already/set"},
			},
			wantCwd: map[string]string{"self": "/already/set"},
		},
		{
			name: "http server never gets a cwd",
			servers: map[string]schema.MCPServerConfig{
				"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
			},
			wantCwd: map[string]string{"atmos-pro": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := withUserScopeCwd(tt.servers, "testdata-project")
			for name, wantCwd := range tt.wantCwd {
				assert.Equal(t, wantCwd, result[name].Cwd, "server %s", name)
			}
		})
	}
}

func newMCPInstallTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)
	return cmd
}

func TestResolveInstallClients_AutoNoFallbackWhenNothingDetected(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set(yesFlag, true)

	clients, err := resolveInstallClients(&schema.AtmosConfiguration{BasePath: base}, mcpinstall.ScopeProject, v)
	require.NoError(t, err)
	assert.Empty(t, clients, "auto mode must not fall back to installing into every supported client")
}

func TestResolveInstallClients_AutoUsesExactlyWhatWasDetected(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".cursor"), 0o755))
	v := viper.New()
	v.Set(yesFlag, true)

	clients, err := resolveInstallClients(&schema.AtmosConfiguration{BasePath: base}, mcpinstall.ScopeProject, v)
	require.NoError(t, err)
	assert.Equal(t, []string{mcpinstall.ClientCursor}, clients)
}

func TestResolveInstallClients_ExplicitClientBypassesDetection(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set(yesFlag, true)
	v.Set("client", []string{"vscode"})

	clients, err := resolveInstallClients(&schema.AtmosConfiguration{BasePath: base}, mcpinstall.ScopeProject, v)
	require.NoError(t, err)
	assert.Equal(t, []string{"vscode"}, clients)
}

func TestResolveInstallClients_AllClientsBypassesDetectionEvenWhenEmpty(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set(yesFlag, true)
	v.Set("all-clients", true)

	clients, err := resolveInstallClients(&schema.AtmosConfiguration{BasePath: base}, mcpinstall.ScopeProject, v)
	require.NoError(t, err)
	assert.ElementsMatch(t, mcpinstall.SupportedClients, clients)
}
