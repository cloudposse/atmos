package manage_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func captureNotices(t *testing.T) *bytes.Buffer {
	t.Helper()
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	return &output
}

func TestRestoreSkipsPathPackagesAndNeverLocksThem(t *testing.T) {
	m, source, directory := setup(t)
	output := captureNotices(t)
	m.Config.Plugins.Packages = map[string]schema.PluginPackage{
		"released": {Source: source, Version: "^1"},
		// A profile adds path beside source and version.
		"layered": {Source: source, Version: "^1", Path: directory},
		// A path package needs neither source nor version.
		"only": {Path: directory},
	}

	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{}))

	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	assert.Contains(t, locked.Spec.Packages, "released")
	assert.NotContains(t, locked.Spec.Packages, "layered", "a path package is never locked")
	assert.NotContains(t, locked.Spec.Packages, "only")
	plain := ansi.Strip(output.String())
	assert.Contains(t, plain, "Skipping plugin layered")
	assert.Contains(t, plain, "Skipping plugin only")
	assert.NotContains(t, plain, "Skipping plugin released")
}

func TestRestoreWithoutThePathLocksTheSamePackage(t *testing.T) {
	m, source, _ := setup(t)
	m.Config.Plugins.Packages = map[string]schema.PluginPackage{"layered": {Source: source, Version: "^1"}}
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{}))
	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	assert.Contains(t, locked.Spec.Packages, "layered", "no profile, no path: the package behaves exactly as before")
}

func TestRestoreOfASingleAliasSkipsItWhenItIsAPathPackage(t *testing.T) {
	m, source, directory := setup(t)
	captureNotices(t)
	m.Config.Plugins.Packages = map[string]schema.PluginPackage{"layered": {Source: source, Version: "^1", Path: directory}}
	require.NoError(t, m.Restore(t.Context(), manage.RestoreOptions{Alias: "layered", Update: true}))
	locked, err := pluginlock.Load(m.Project)
	require.NoError(t, err)
	assert.Empty(t, locked.Spec.Packages)
}
