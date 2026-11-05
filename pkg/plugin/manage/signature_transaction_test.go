package manage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFailedSignatureVerificationPreservesProjectTransaction(t *testing.T) {
	manager, source, _ := setup(t)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "existing", Source: source, Constraint: "1.0.0"}))
	configPath := filepath.Join(manager.Project, manager.ConfigFile)
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(before, manager.Config))
	lockBefore, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	manager.Config.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "example", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	err = manager.Add(t.Context(), manage.AddOptions{Alias: "rejected", Source: source, Constraint: "1.0.0"})
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	lockAfter, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	require.Equal(t, lockBefore, lockAfter)
	require.Contains(t, manager.Config.Plugins.Packages, "existing")
	require.NotContains(t, manager.Config.Plugins.Packages, "rejected")
}
