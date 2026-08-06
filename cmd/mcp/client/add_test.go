package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpconfig "github.com/cloudposse/atmos/pkg/mcp/config"
	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAddCmd_Registration(t *testing.T) {
	assert.Equal(t, "add [preset-name|url|command] [flags]", addCmd.Use)
	assert.NotEmpty(t, addCmd.Short)
	assert.NotEmpty(t, addCmd.Long)
	assert.NotNil(t, addCmd.RunE)

	for _, name := range []string{"name", "transport", "env", "header", "description", "identity", "timeout", "auto-start", "install", "yes", "force"} {
		assert.NotNil(t, addCmd.Flags().Lookup(name), "expected %s flag", name)
	}
}

func TestResolveAddTarget(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantTarget    string
		wantDefaulted bool
	}{
		{name: "no args defaults to self preset", args: nil, wantTarget: mcpconfig.PresetSelf, wantDefaulted: true},
		{name: "explicit target is used as-is", args: []string{"atmos-pro"}, wantTarget: "atmos-pro", wantDefaulted: false},
		{name: "explicit url", args: []string{"https://mcp.example.com/mcp"}, wantTarget: "https://mcp.example.com/mcp", wantDefaulted: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, defaulted := resolveAddTarget(tt.args)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantDefaulted, defaulted)
		})
	}
}

func TestConfirmAddOverwrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("mcp:\n  servers:\n    existing:\n      command: a\n"), 0o600))

	t.Run("force bypasses prompt for existing entry", func(t *testing.T) {
		proceed, err := confirmAddOverwrite(file, "existing", false, true)
		require.NoError(t, err)
		assert.True(t, proceed)
	})

	t.Run("yes bypasses prompt for existing entry", func(t *testing.T) {
		proceed, err := confirmAddOverwrite(file, "existing", true, false)
		require.NoError(t, err)
		assert.True(t, proceed)
	})

	t.Run("new name never prompts", func(t *testing.T) {
		proceed, err := confirmAddOverwrite(file, "brand-new", false, false)
		require.NoError(t, err)
		assert.True(t, proceed)
	})
}

func TestEnsureMCPEnabledForPreset(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", nil, "")

	t.Run("non-preset target is a no-op", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := ensureMCPEnabledForPreset(cmd, atmosConfig, "https://mcp.example.com/mcp", true)
		require.NoError(t, err)
	})

	t.Run("atmos-pro preset doesn't require mcp.enabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := ensureMCPEnabledForPreset(cmd, atmosConfig, mcpconfig.PresetAtmosPro, true)
		require.NoError(t, err)
	})

	t.Run("self preset already enabled is a no-op", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{Enabled: true},
		}
		err := ensureMCPEnabledForPreset(cmd, atmosConfig, mcpconfig.PresetSelf, true)
		require.NoError(t, err)
	})

	t.Run("self preset disabled and yes=true hard-errors without mutating", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := ensureMCPEnabledForPreset(cmd, atmosConfig, mcpconfig.PresetSelf, true)
		require.Error(t, err)
		assert.False(t, atmosConfig.MCP.Enabled)
	})
}

func TestInstallAfterAdd_NoClientsDetectedSkipsWithoutError(t *testing.T) {
	base := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: base}

	err := installAfterAdd(atmosConfig, "atmos", &schema.MCPServerConfig{Command: "atmos", Args: []string{"mcp", "start"}}, true, false)
	require.NoError(t, err, "no detected clients must not be an error")
}

func TestInstallAfterAdd_InstallsIntoDetectedClient(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".cursor"), 0o755))
	atmosConfig := &schema.AtmosConfiguration{BasePath: base}

	err := installAfterAdd(atmosConfig, "atmos", &schema.MCPServerConfig{Command: "atmos", Args: []string{"mcp", "start"}}, true, false)
	require.NoError(t, err)

	target, err := mcpinstall.ResolveTarget(base, "", mcpinstall.ScopeProject, mcpinstall.ClientCursor)
	require.NoError(t, err)
	data, err := os.ReadFile(target.Path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "atmos")
}
