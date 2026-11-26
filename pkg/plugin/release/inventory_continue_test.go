package release

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
)

// expectIsolatedSource expects the source resolution, worktree creation, and cleanup of one package.
func expectIsolatedSource(t *testing.T, runner *MockRunner, entry releaseplan.Entry) {
	t.Helper()
	runner.EXPECT().Run(gomock.Any(), "git", []string{"rev-parse", "--verify", "--end-of-options", entry.Source + "^{commit}"}, &pkgit.RunOptions{}).Return(pkgit.RunResult{Stdout: entry.Source}, nil)
	runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), &pkgit.RunOptions{}).DoAndReturn(func(_ context.Context, _ string, args []string, _ *pkgit.RunOptions) (pkgit.RunResult, error) {
		require.Equal(t, []string{"worktree", "add", "--detach"}, args[:3])
		require.Equal(t, entry.Source, args[4])
		return pkgit.RunResult{}, os.Mkdir(args[3], 0o700)
	})
	runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), &pkgit.RunOptions{}).DoAndReturn(func(cleanup context.Context, _ string, args []string, _ *pkgit.RunOptions) (pkgit.RunResult, error) {
		require.NoError(t, cleanup.Err(), "every started package must remove its worktree")
		require.Equal(t, []string{"worktree", "remove", "--force"}, args[:3])
		return pkgit.RunResult{}, nil
	})
}

// A failing package must not block the packages after it (#3763): the others are still
// released, and the error names only the package that failed.
func TestInventoryContinuesPastAFailingPackage(t *testing.T) {
	entries := []releaseplan.Entry{
		{Name: "first", Path: "plugins/first", Source: "first-source"},
		{Name: "broken", Path: "plugins/broken", Source: "broken-source"},
		{Name: "last", Path: "plugins/last", Source: "last-source"},
	}
	tests := []struct {
		name string
		// Options and entries started for the run.
		failFast bool
		started  []releaseplan.Entry
		// Expected outcome.
		released   []string
		incomplete string
	}{
		{name: "continues by default", started: entries, released: []string{"first", "last"}, incomplete: "1 of 3 packages did not complete"},
		{name: "fail fast stops at the first failure", failFast: true, started: entries[:2], released: []string{"first"}, incomplete: "2 of 3 packages did not complete"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runner := NewMockRunner(ctrl)
			parent := t.TempDir()
			for _, entry := range test.started {
				expectIsolatedSource(t, runner, entry)
			}
			factory := func(entry releaseplan.Entry, _ string) Backend {
				backend := NewMockBackend(ctrl)
				if entry.Name == "broken" {
					backend.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(errUtils.ErrPluginBuild)
					return backend
				}
				gomock.InOrder(backend.EXPECT().Validate(gomock.Any(), gomock.Any()), backend.EXPECT().Build(gomock.Any(), gomock.Any()), backend.EXPECT().Test(gomock.Any(), gomock.Any()), backend.EXPECT().Pack(gomock.Any(), gomock.Any(), gomock.Any()))
				backend.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(&pluginoci.Release{Name: entry.Name}, nil)
				return backend
			}

			results, err := RunInventory(t.Context(), entries, factory, WithInventoryRunner(runner), WithInventoryTemporaryDirectory(parent), WithInventoryFailFast(test.failFast))

			require.ErrorIs(t, err, errUtils.ErrPluginInventoryRelease)
			require.ErrorIs(t, err, errUtils.ErrPluginBuild)
			require.Contains(t, err.Error(), "plugin release broken:")
			require.Contains(t, err.Error(), test.incomplete)
			require.NotContains(t, err.Error(), "plugin release first")
			require.NotContains(t, err.Error(), "plugin release last")
			released := make([]string, 0, len(results))
			for _, result := range results {
				require.Equal(t, result.Entry.Name, result.Release.Name, "each release keeps the entry that produced it")
				released = append(released, result.Entry.Name)
			}
			require.Equal(t, test.released, released)
			remaining, err := os.ReadDir(parent)
			require.NoError(t, err)
			require.Empty(t, remaining)
		})
	}
}

// Cancellation stops the run even without fail-fast, and no later package starts.
func TestInventoryCancellationStopsWithoutFailFast(t *testing.T) {
	ctrl := gomock.NewController(t)
	runner := NewMockRunner(ctrl)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entries := []releaseplan.Entry{
		{Name: "first", Path: "plugins/first", Source: "first-source"},
		{Name: "cancelled", Path: "plugins/cancelled", Source: "cancelled-source"},
		{Name: "never", Path: "plugins/never", Source: "never-source"},
	}
	expectIsolatedSource(t, runner, entries[0])
	expectIsolatedSource(t, runner, entries[1])
	var started []string
	factory := func(entry releaseplan.Entry, _ string) Backend {
		started = append(started, entry.Name)
		backend := NewMockBackend(ctrl)
		if entry.Name == "cancelled" {
			backend.EXPECT().Validate(gomock.Any(), gomock.Any()).Do(func(context.Context, string) { cancel() })
			return backend
		}
		gomock.InOrder(backend.EXPECT().Validate(gomock.Any(), gomock.Any()), backend.EXPECT().Build(gomock.Any(), gomock.Any()), backend.EXPECT().Test(gomock.Any(), gomock.Any()), backend.EXPECT().Pack(gomock.Any(), gomock.Any(), gomock.Any()))
		backend.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(&pluginoci.Release{Name: entry.Name}, nil)
		return backend
	}

	results, err := RunInventory(ctx, entries, factory, WithInventoryRunner(runner), WithInventoryTemporaryDirectory(t.TempDir()))

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrPluginInventoryRelease)
	require.Contains(t, err.Error(), "2 of 3 packages did not complete")
	require.Equal(t, []string{"first", "cancelled"}, started, "no package starts after cancellation")
	require.Len(t, results, 1)
	require.Equal(t, "first", results[0].Entry.Name)
}

// An inventory in which every package succeeds reports no error.
func TestInventoryErrorIsNilWithoutFailures(t *testing.T) {
	require.NoError(t, inventoryError(0, 2, nil))
	err := inventoryError(1, 2, []error{errUtils.ErrPluginBuild})
	require.ErrorIs(t, err, errUtils.ErrPluginInventoryRelease)
	require.ErrorIs(t, err, errUtils.ErrPluginBuild)
	require.Equal(t, "plugin inventory release failed: 1 of 2 packages did not complete\nplugin build failed", err.Error())
}
