package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestReplacementRollback(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		failInstall, failUninstall, failRestore bool
	}{
		{name: "success"},
		{name: "install failure", failInstall: true},
		{name: "uninstall failure", failUninstall: true},
		{name: "restore failure retains backup", failInstall: true, failRestore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "plugins")
			previous := seedPlugin(t, dir, "sample", "1.0.0", nil)
			originalBinary, statErr := os.Stat(filepath.Join(previous, "binary"))
			require.NoError(t, statErr)
			runner := &fakeRunner{}
			if tc.failInstall {
				runner.installErr = os.ErrPermission
			}
			if tc.failUninstall {
				runner.hook = func(_ context.Context, args []string, _ string) (bool, string, string, error) {
					if args[1] == "uninstall" {
						return true, "", "denied", os.ErrPermission
					}
					return false, "", "", nil
				}
			}
			inst := newTestInstaller(runner, dir)
			if tc.failRestore {
				inst.rename = func(_, _ string) error { return os.ErrPermission }
			}
			_, err := inst.EnsurePlugins(context.Background(), []Spec{testPlugin})
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
				binary, statErr := os.Stat(backups[0])
				require.NoError(t, statErr)
				assert.Equal(t, originalBinary.Mode(), binary.Mode())
			case tc.failInstall || tc.failUninstall:
				require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
				data, readErr := os.ReadFile(filepath.Join(previous, "binary"))
				require.NoError(t, readErr)
				assert.Equal(t, "previous binary", string(data))
				binary, statErr := os.Stat(filepath.Join(previous, "binary"))
				require.NoError(t, statErr)
				assert.Equal(t, originalBinary.Mode(), binary.Mode())
			default:
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(dir, "sample", installReceiptName))
			}
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
	expected := seedPlugin(t, dir, "sample", "1.0.0", nil)
	got, err := findPluginDirectory(dir, "sample")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	got, err = findPluginDirectory(dir, "other")
	require.NoError(t, err)
	assert.Empty(t, got)
	require.NoError(t, os.WriteFile(filepath.Join(expected, "plugin.yaml"), []byte("name: ["), 0o644))
	_, err = findPluginDirectory(dir, "sample")
	require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
	_, err = newTestInstaller(&fakeRunner{}, dir).backupPlugin("sample")
	require.Error(t, err)
	_, err = findPluginDirectory(filepath.Join(dir, "missing"), "sample")
	require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
}
