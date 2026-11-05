package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPEnvironmentKeyCase(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)
	dir := t.TempDir()
	t.Setenv("MCP_CASE_VALUE", "resolved")
	files := map[string]string{
		"base.yaml": `mcp:
  servers:
    local:
      command: python3
      env:
        INHERITED: base
        SHARED: base
`,
		"atmos.yaml": `import: [base.yaml]
mcp:
  servers:
    local:
      env:
        UPPER: !env MCP_CASE_VALUE
        upper: lowercase
        SHARED: override
    alias:
      command: python3
      env: &environment
        ALIAS_KEY: value
    alias_copy:
      command: python3
      env: *environment
`,
		"env.yaml": "INCLUDED_KEY: value\nincluded_key: lower\n",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	cfg := loadConfigViaMergeConfig(t, dir)
	require.Equal(t, map[string]string{"INHERITED": "base", "SHARED": "override", "UPPER": "resolved", "upper": "lowercase"}, cfg.MCP.Servers["local"].Env)
	require.Equal(t, map[string]string{"ALIAS_KEY": "value"}, cfg.MCP.Servers["alias"].Env)
	require.Equal(t, cfg.MCP.Servers["alias"].Env, cfg.MCP.Servers["alias_copy"].Env)
}

func TestMCPIncludedEnvironmentKeyCase(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("mcp:\n  servers:\n    local:\n      command: python3\n      env: !include env.yaml\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "env.yaml"), []byte("INCLUDED_KEY: value\nincluded_key: lower\n"), 0o600))
	cfg := loadConfigViaMergeConfig(t, dir)
	require.Equal(t, map[string]string{"INCLUDED_KEY": "value", "included_key": "lower"}, cfg.MCP.Servers["local"].Env)
}
