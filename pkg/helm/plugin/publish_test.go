package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func writePreviousPlugin(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "helm-diff")
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "plugin.yaml"), []byte("name: diff\nversion: 3.8.0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(path, "binary"), []byte("previous binary"), 0o755))
	return path
}

func TestPublishReplacementRollback(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		failBackup, failPublish, failRestore bool
	}{
		{name: "success"},
		{name: "backup failure", failBackup: true},
		{name: "publish failure", failPublish: true},
		{name: "restore failure retains backup", failPublish: true, failRestore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "plugins")
			previous := writePreviousPlugin(t, dir)
			runner := &fakeRunner{listOutput: "NAME VERSION\ndiff 3.8.0\n"}
			inst := newTestInstaller(runner, dir)
			inst.rename = func(from, to string) error {
				switch {
				case tc.failBackup && from == previous:
					return os.ErrPermission
				case tc.failPublish && strings.HasPrefix(filepath.Base(filepath.Dir(from)), ".helm-plugin-install-"):
					return os.ErrPermission
				case tc.failRestore && strings.HasPrefix(filepath.Base(filepath.Dir(from)), ".helm-plugin-rollback-"):
					return os.ErrPermission
				default:
					return os.Rename(from, to)
				}
			}
			_, err := inst.EnsurePlugins(context.Background(), []Spec{pinnedDiff})
			switch {
			case tc.failRestore:
				require.ErrorContains(t, err, "rollback failed")
				backups, globErr := filepath.Glob(filepath.Join(parent, ".helm-plugin-rollback-*", "plugin", "binary"))
				require.NoError(t, globErr)
				require.Len(t, backups, 1)
				data, readErr := os.ReadFile(backups[0])
				require.NoError(t, readErr)
				assert.Equal(t, "previous binary", string(data))
				require.ErrorContains(t, err, filepath.Dir(backups[0]))
			case tc.failBackup || tc.failPublish:
				require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
				data, readErr := os.ReadFile(filepath.Join(previous, "binary"))
				require.NoError(t, readErr)
				assert.Equal(t, "previous binary", string(data))
			default:
				require.NoError(t, err)
				require.NoDirExists(t, previous)
				require.FileExists(t, filepath.Join(dir, "diff", "plugin.yaml"))
			}
			stages, globErr := filepath.Glob(filepath.Join(parent, ".helm-plugin-install-*"))
			require.NoError(t, globErr)
			assert.Empty(t, stages)
			if !tc.failRestore {
				backups, globErr := filepath.Glob(filepath.Join(parent, ".helm-plugin-rollback-*"))
				require.NoError(t, globErr)
				assert.Empty(t, backups)
			}
		})
	}
}

func TestFindPluginDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".install.lock"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o755))
	expected := writePreviousPlugin(t, dir)
	got, err := findPluginDirectory(dir, "diff")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	got, err = findPluginDirectory(dir, "other")
	require.NoError(t, err)
	assert.Empty(t, got)
	require.NoError(t, os.WriteFile(filepath.Join(expected, "plugin.yaml"), []byte("name: ["), 0o644))
	_, err = findPluginDirectory(dir, "diff")
	require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
	inst := newTestInstaller(&fakeRunner{}, dir)
	require.Error(t, inst.publishInstall("unused", "diff"))
	_, err = findPluginDirectory(filepath.Join(dir, "missing"), "diff")
	require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
}
