package configuration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// A project root reached through a symlink still applies its exclusions to the resolved files.
func TestUnexcludedWatchFilesResolveRootAliases(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	require.NoError(t, os.Mkdir(root, 0o755))
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cache := filepath.Join(root, "live", ".terragrunt-cache", "deleted.tf")
	ordinary := filepath.Join(root, "live", "main.tf")
	files, err := UnexcludedWatchFiles(alias, []string{cache, ordinary}, []string{"**/.terragrunt-cache/**"})
	require.NoError(t, err)
	require.Equal(t, []string{ordinary}, files)
}

// A changed file outside the project cannot match a project-relative exclusion, even `**`.
func TestUnexcludedWatchFilesKeepFilesOutsideTheProject(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	sibling := filepath.Join(parent, "sibling", "main.tf")
	inside := filepath.Join(root, "live", "main.tf")
	files, err := UnexcludedWatchFiles(root, []string{sibling, inside}, []string{"**"})
	require.NoError(t, err)
	require.Equal(t, []string{sibling}, files)

	_, err = UnexcludedWatchFiles("relative", []string{inside}, []string{"**"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, "a relative root would silently match nothing")
}

func TestProjectRelativeTreatsUnrelatablePathsAsOutside(t *testing.T) {
	root := t.TempDir()
	relative, inside := projectRelative(root, filepath.Join(root, "a", "b.tf"))
	require.True(t, inside)
	require.Equal(t, filepath.Join("a", "b.tf"), relative)
	_, inside = projectRelative(root, filepath.Dir(root))
	require.False(t, inside)
	// filepath.Rel fails when it cannot relate the two paths; the file is outside, not an error.
	_, inside = projectRelative(root, "relative.tf")
	require.False(t, inside)
	if runtime.GOOS == "windows" {
		other := `Z:\elsewhere\main.tf`
		if filepath.VolumeName(root) == "Z:" {
			other = `Y:\elsewhere\main.tf`
		}
		_, inside = projectRelative(root, other)
		require.False(t, inside, "a file on another volume is outside the project")
		files, err := UnexcludedWatchFiles(root, []string{other}, []string{"**"})
		require.NoError(t, err)
		require.Equal(t, []string{other}, files)
	}
}
