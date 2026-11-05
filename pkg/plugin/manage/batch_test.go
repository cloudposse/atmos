package manage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFailedRestoreBatchPreservesProjectFiles(t *testing.T) {
	manager, source, _ := setup(t)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "existing", Source: source, Constraint: "1.0.0"}))
	configBefore, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
	require.NoError(t, err)
	lockBefore, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	manager.Config.Plugins.MaxConcurrency = 2
	manager.Config.Plugins.Packages = map[string]schema.PluginPackage{
		"existing":    {Source: source, Version: "1.0.0"},
		"new":         {Source: source, Version: "1.0.0"},
		"unavailable": {Source: source, Version: "2.0.0"},
	}
	require.Error(t, manager.Restore(t.Context(), manage.RestoreOptions{}))
	configAfter, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
	require.NoError(t, err)
	lockAfter, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	require.Equal(t, configBefore, configAfter)
	require.Equal(t, lockBefore, lockAfter)
	file, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	require.Len(t, file.Spec.Packages, 1)
	require.Contains(t, file.Spec.Packages, "existing")
}
