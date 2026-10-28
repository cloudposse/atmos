package manage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestUpdateFeedbackDistinguishesPinUnchangedAndNewRelease(t *testing.T) {
	manager, source, directory := setup(t)
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "corporate", Source: source, Constraint: "1.0.0"}))
	reloadConfiguration(t, manager)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	require.NoError(t, manager.Restore(t.Context(), manage.RestoreOptions{Update: true, Alias: "corporate"}))
	require.Contains(t, ansi.Strip(output.String()), "corporate")
	require.Contains(t, ansi.Strip(output.String()), "already up to date")
	require.Contains(t, ansi.Strip(output.String()), "1.0.0")
	output.Reset()

	definition, err := os.ReadFile(filepath.Join(directory, "plugin.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(string(definition), "version: 1.0.0", "version: 1.1.0")), 0o644))
	index, err := pluginoci.BuildIndex(directory, manager.Installer.Descriptors)
	require.NoError(t, err)
	repository, err := name.NewRepository(strings.TrimPrefix(source, "oci://"))
	require.NoError(t, err)
	require.NoError(t, manager.Installer.Registry.WriteIndex(t.Context(), repository.Tag("1.1.0"), index))
	require.NoError(t, manager.Restore(t.Context(), manage.RestoreOptions{Update: true, Alias: "corporate"}))
	require.Contains(t, ansi.Strip(output.String()), "already up to date", "exact pins remain unchanged even when newer versions exist")
	output.Reset()
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "corporate", Source: source, Constraint: "^1.0"}))
	plain := ansi.Strip(output.String())
	require.Contains(t, plain, "Updated plugin")
	require.Contains(t, plain, "1.0.0")
	require.Contains(t, plain, "1.1.0", "previous metadata must not constrain the new version selection")
	require.NotContains(t, plain, "already up to date")
}

func TestFailedUpdateReturnsContextWithoutPrintingDuplicateError(t *testing.T) {
	manager, source, _ := setup(t)
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("TERM", "dumb")
	context, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(context)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	err = manager.Add(t.Context(), manage.AddOptions{Alias: "corporate", Source: source, Constraint: "^2.0"})
	require.ErrorIs(t, err, errUtils.ErrPluginVersionNotFound)
	// The batch renderer shows the failure with its context and hints; the returned error is
	// marked as reported so the root handler exits without printing it a second time.
	require.True(t, reported.IsReported(err))
	plain := ansi.Strip(output.String())
	require.Equal(t, 1, strings.Count(plain, "no plugin release satisfies"), "the failure is rendered once")
	require.Contains(t, plain, "✗ corporate")
	require.Contains(t, plain, "^2.0")
	require.Contains(t, plain, "atmos plugin search")
	require.Contains(t, plain, "No plugin changes were written.")
}
