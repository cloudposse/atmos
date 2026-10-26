package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestInitCliConfig_MCPEnabledSurvivesAtmosDMerge is a regression test for a
// field-test finding: MCPSettings.Enabled used to be a bare `bool` tagged
// `omitempty`, which can't represent an explicit `false` distinctly from
// "unset". Merging a root atmos.yaml (`mcp.enabled: false`) over an atmos.d
// fragment (`mcp.enabled: true`) made the field vanish from the merged struct
// entirely -- not false, not true, just absent -- rather than the root's
// explicit false correctly winning.
func TestInitCliConfig_MCPEnabledSurvivesAtmosDMerge(t *testing.T) {
	tmpDir := t.TempDir()

	atmosYAML := `
base_path: "./"
logs:
  file: "/dev/stderr"
  level: Info
mcp:
  enabled: false
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	atmosDDir := filepath.Join(tmpDir, "atmos.d")
	require.NoError(t, os.MkdirAll(atmosDDir, 0o755))
	fragment := `
mcp:
  enabled: true
`
	require.NoError(t, os.WriteFile(filepath.Join(atmosDDir, "extra.yaml"), []byte(fragment), 0o644))

	changeWorkingDir(t, tmpDir)

	cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	require.NotNil(t, cfg.MCP.Enabled, "mcp.enabled must survive the atmos.d merge, not vanish to nil/absent")
	require.False(t, cfg.MCP.IsEnabled(), "root atmos.yaml's explicit false must win over atmos.d's true")
}
