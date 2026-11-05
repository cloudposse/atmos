package release

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
)

func TestInventoryUsesRecordedSourceAndCleansEachPackage(t *testing.T) {
	for _, check := range []bool{false, true} {
		t.Run(map[bool]string{true: "check", false: "publish"}[check], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runner := NewMockRunner(ctrl)
			parent := t.TempDir()
			entries := []releaseplan.Entry{{Name: "first", Path: "plugins/first", Source: "first-source"}, {Name: "second", Path: "plugins/second", Source: "second-source"}}
			var factoryEntries []releaseplan.Entry
			expected := []*pluginoci.Release{{Name: "first"}, {Name: "second"}}
			for index, entry := range entries {
				runner.EXPECT().Run(gomock.Any(), "git", []string{"rev-parse", "--verify", "--end-of-options", entry.Source + "^{commit}"}, pkgit.RunOptions{}).Return(pkgit.RunResult{Stdout: entry.Source + "\n"}, nil)
				runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).DoAndReturn(func(_ context.Context, _ string, args []string, _ pkgit.RunOptions) (pkgit.RunResult, error) {
					require.Equal(t, []string{"worktree", "add", "--detach"}, args[:3])
					require.Equal(t, entry.Source, args[4])
					require.NoDirExists(t, args[3])
					return pkgit.RunResult{}, os.Mkdir(args[3], 0o700)
				})
				runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).DoAndReturn(func(ctx context.Context, _ string, args []string, _ pkgit.RunOptions) (pkgit.RunResult, error) {
					require.NoError(t, ctx.Err())
					require.Equal(t, []string{"worktree", "remove", "--force"}, args[:3])
					return pkgit.RunResult{}, nil
				})
				_ = index
			}
			factory := func(entry releaseplan.Entry, project string) Backend {
				index := len(factoryEntries)
				factoryEntries = append(factoryEntries, entry)
				require.DirExists(t, project)
				backend := NewMockBackend(ctrl)
				directory := filepath.Join(project, filepath.FromSlash(entry.Path))
				gomock.InOrder(backend.EXPECT().Validate(gomock.Any(), directory), backend.EXPECT().Build(gomock.Any(), directory), backend.EXPECT().Test(gomock.Any(), directory), backend.EXPECT().Pack(gomock.Any(), directory, gomock.Any()))
				if !check {
					backend.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(expected[index], nil)
				}
				return backend
			}
			actual, err := RunInventory(t.Context(), entries, factory, WithInventoryRunner(runner), WithInventoryTemporaryDirectory(parent), WithInventoryCheck(check))
			require.NoError(t, err)
			require.Equal(t, entries, factoryEntries)
			if check {
				require.Equal(t, []*pluginoci.Release{nil, nil}, actual)
			} else {
				require.Equal(t, expected, actual)
			}
			remaining, err := os.ReadDir(parent)
			require.NoError(t, err)
			require.Empty(t, remaining)
		})
	}
}

func TestInventoryFailuresAlwaysCleanUpAndStop(t *testing.T) {
	for _, phase := range []string{"resolve", "empty revision", "add", "validate", "cancel", "cleanup"} {
		t.Run(phase, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runner := NewMockRunner(ctrl)
			backend := NewMockBackend(ctrl)
			parent := t.TempDir()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			entry := releaseplan.Entry{Name: "example", Path: "plugins/example", Source: "source"}
			expected := errUtils.ErrPluginBuild
			resolve := runner.EXPECT().Run(gomock.Any(), "git", []string{"rev-parse", "--verify", "--end-of-options", "source^{commit}"}, pkgit.RunOptions{})
			switch phase {
			case "resolve":
				resolve.Return(pkgit.RunResult{}, expected)
			case "empty revision":
				resolve.Return(pkgit.RunResult{}, nil)
				expected = errUtils.ErrPluginInvalidConfig
			default:
				resolve.Return(pkgit.RunResult{Stdout: "source"}, nil)
				runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).DoAndReturn(func(_ context.Context, _ string, args []string, _ pkgit.RunOptions) (pkgit.RunResult, error) {
					require.Equal(t, []string{"worktree", "add", "--detach"}, args[:3])
					if phase == "add" {
						return pkgit.RunResult{}, expected
					}
					return pkgit.RunResult{}, nil
				})
				runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).DoAndReturn(func(cleanup context.Context, _ string, args []string, _ pkgit.RunOptions) (pkgit.RunResult, error) {
					require.NoError(t, cleanup.Err(), "cancelled releases must still remove worktrees")
					require.Equal(t, []string{"worktree", "remove", "--force"}, args[:3])
					if phase == "cleanup" {
						return pkgit.RunResult{}, expected
					}
					return pkgit.RunResult{}, nil
				})
				switch phase {
				case "validate":
					backend.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(expected)
				case "cancel":
					backend.EXPECT().Validate(gomock.Any(), gomock.Any()).Do(func(context.Context, string) { cancel() })
				case "cleanup":
					backend.EXPECT().Validate(gomock.Any(), gomock.Any())
					backend.EXPECT().Build(gomock.Any(), gomock.Any())
					backend.EXPECT().Test(gomock.Any(), gomock.Any())
					backend.EXPECT().Pack(gomock.Any(), gomock.Any(), gomock.Any())
				}
			}
			results, err := RunInventory(ctx, []releaseplan.Entry{entry, {Name: "must-not-run"}}, func(releaseplan.Entry, string) Backend { return backend }, WithInventoryRunner(runner), WithInventoryTemporaryDirectory(parent), WithInventoryCheck(true))
			require.Nil(t, results)
			if phase == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, expected)
			}
			remaining, err := os.ReadDir(parent)
			require.NoError(t, err)
			require.Empty(t, remaining)
		})
	}
}

func TestInventoryRejectsInvalidInputs(t *testing.T) {
	factory := func(releaseplan.Entry, string) Backend { return nil }
	_, err := RunInventory(t.Context(), nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = RunInventory(t.Context(), nil, factory, WithInventoryRunner(nil))
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	for _, entry := range []releaseplan.Entry{{Path: "../escape", Source: "source"}, {Path: "valid"}} {
		_, err = RunInventory(t.Context(), []releaseplan.Entry{entry}, factory)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = RunInventory(ctx, []releaseplan.Entry{{}}, factory)
	require.ErrorIs(t, err, context.Canceled)
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), "git", gomock.Any(), pkgit.RunOptions{}).Return(pkgit.RunResult{Stdout: "source"}, nil)
	_, err = RunInventory(t.Context(), []releaseplan.Entry{{Path: "valid", Source: "source"}}, factory, WithInventoryRunner(runner), WithInventoryTemporaryDirectory(filepath.Join(t.TempDir(), "absent")))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCurrentInventoryHeadFetchesCurrentDefaultBranch(t *testing.T) {
	for _, phase := range []string{"success", "fetch", "resolve", "empty", "missing runner"} {
		t.Run(phase, func(t *testing.T) {
			runner := NewMockRunner(gomock.NewController(t))
			failure := errUtils.ErrGitCommandExited
			if phase == "missing runner" {
				_, err := CurrentInventoryHead(t.Context(), nil)
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				return
			}
			fetch := runner.EXPECT().Run(gomock.Any(), "git", []string{"fetch", "--no-tags", "origin", "HEAD"}, pkgit.RunOptions{})
			if phase == "fetch" {
				fetch.Return(pkgit.RunResult{}, failure)
			} else {
				resolve := runner.EXPECT().Run(gomock.Any(), "git", []string{"rev-parse", "--verify", "FETCH_HEAD^{commit}"}, pkgit.RunOptions{})
				switch phase {
				case "success":
					resolve.Return(pkgit.RunResult{Stdout: "latest-accepted-commit\n"}, nil)
				case "resolve":
					resolve.Return(pkgit.RunResult{}, failure)
				case "empty":
					resolve.Return(pkgit.RunResult{}, nil)
				}
			}
			head, err := CurrentInventoryHead(t.Context(), runner)
			switch phase {
			case "success":
				require.NoError(t, err)
				require.Equal(t, "latest-accepted-commit", head)
			case "empty":
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			default:
				require.ErrorIs(t, err, failure)
			}
		})
	}
}
