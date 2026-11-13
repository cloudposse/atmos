package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestClearStaleSignalsUsesProvisionedModuleDirectory(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	section := map[string]any{
		"component": "example", "atmos_stack": "dev",
		"source":    map[string]any{"uri": "git::https://example.test/repo.git"},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}
	info := &schema.ConfigAndStacksInfo{ComponentSection: section, FinalComponent: "example", Stack: "dev", BaseComponentPath: "module"}
	target, err := source.ResolveTarget(atmosConfig, "terraform", "example", section)
	require.NoError(t, err)
	moduleDir := filepath.Join(target.Dir, "module")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	stale := filepath.Join(moduleDir, "error-signals.json")
	require.NoError(t, os.WriteFile(stale, []byte(`{"old":true}`), 0o644))
	require.NoError(t, clearStaleSignals(atmosConfig, info))
	require.NoFileExists(t, stale)

	info.DryRun = true
	require.NoError(t, os.WriteFile(stale, []byte(`{"old":true}`), 0o644))
	require.NoError(t, clearStaleSignals(atmosConfig, info))
	require.FileExists(t, stale, "dry runs leave the module untouched")
}
