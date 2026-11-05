package manage_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestRestoreSkipsDisabledPackagesAndSaysSo(t *testing.T) {
	manager, source, _ := setup(t)
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	off := false
	manager.Config.Plugins.Packages = map[string]schema.PluginPackage{"off": {Source: source, Version: "^1", Enabled: &off}}
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))

	require.NoError(t, manager.Restore(t.Context(), manage.RestoreOptions{}))

	locked, err := pluginlock.Load(manager.Project)
	require.NoError(t, err)
	assert.Empty(t, locked.Spec.Packages, "a disabled package must not be installed")
	plain := ansi.Strip(output.String())
	assert.Contains(t, plain, "off")
	assert.Contains(t, plain, "disabled")

	output.Reset()
	require.NoError(t, manager.Restore(t.Context(), manage.RestoreOptions{Alias: "off"}))
	assert.Contains(t, ansi.Strip(output.String()), "disabled", "naming a disabled package explicitly says it is disabled")
	locked, err = pluginlock.Load(manager.Project)
	require.NoError(t, err)
	assert.Empty(t, locked.Spec.Packages)
}

func TestFrozenRestoreIgnoresDisabledPackages(t *testing.T) {
	manager, source, _ := setup(t)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source, Constraint: "^1"}))
	reloadConfiguration(t, manager)
	off := false
	manager.Config.Plugins.Packages["off"] = schema.PluginPackage{Source: source, Version: "^1", Enabled: &off}
	require.NoError(t, manager.Restore(t.Context(), manage.RestoreOptions{Frozen: true}))
}

func TestFailuresCarryActionableHints(t *testing.T) {
	manager, source, _ := setup(t)
	err := manager.Remove(t.Context(), "missing")
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "atmos plugin")
	err = manager.Add(t.Context(), manage.AddOptions{Alias: "Bad Alias", Source: source})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "lowercase")
	err = manager.Restore(t.Context(), manage.RestoreOptions{Alias: "missing"})
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "atmos plugin")
}
