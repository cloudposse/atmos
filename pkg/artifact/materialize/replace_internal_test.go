//go:build linux || darwin

package materialize

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestExchangeDestinationFailurePreservesOriginal(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(scratch, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(scratch, "tree"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "original"), []byte("original"), 0o600))
	restricted := filepath.Join(dest, "restricted")
	require.NoError(t, os.Mkdir(restricted, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(restricted, "keep"), []byte("keep"), 0o600))
	require.NoError(t, os.Chmod(restricted, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restricted, 0o700) })
	stageInfo, err := os.Lstat(filepath.Join(scratch, "tree"))
	require.NoError(t, err)
	destInfo, err := os.Lstat(dest)
	require.NoError(t, err)
	stageParent, err := os.Open(scratch)
	require.NoError(t, err)
	defer stageParent.Close()
	destParent, err := os.Open(root)
	require.NoError(t, err)
	defer destParent.Close()
	sentinel := errors.New("exchange failed")
	preserve, err := exchangeDestination(&exchangePair{stageParent: stageParent, stage: "tree", destParent: destParent, dest: "dest", stageInfo: stageInfo, destInfo: destInfo}, func(*os.File, string, *os.File, string) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	require.False(t, preserve)
	data, err := os.ReadFile(filepath.Join(dest, "original"))
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	require.NoError(t, os.Chmod(restricted, 0o700))
	data, err = os.ReadFile(filepath.Join(restricted, "keep"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

func TestExchangeDestinationRejectsLastMomentSwap(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(scratch, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(scratch, "tree"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "original"), []byte("original"), 0o600))
	stageInfo, err := os.Lstat(filepath.Join(scratch, "tree"))
	require.NoError(t, err)
	destInfo, err := os.Lstat(dest)
	require.NoError(t, err)
	stageParent, err := os.Open(scratch)
	require.NoError(t, err)
	defer stageParent.Close()
	destParent, err := os.Open(root)
	require.NoError(t, err)
	defer destParent.Close()
	calls := 0
	swap := func(a *os.File, b string, c *os.File, d string) error {
		calls++
		if calls == 1 {
			require.NoError(t, os.Rename(dest, filepath.Join(root, "original")))
			require.NoError(t, os.Mkdir(dest, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dest, "intruder"), []byte("intact"), 0o600))
		}
		return exchangeDirectories(a, b, c, d)
	}
	preserve, err := exchangeDestination(&exchangePair{stageParent: stageParent, stage: "tree", destParent: destParent, dest: "dest", stageInfo: stageInfo, destInfo: destInfo}, swap)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
	require.False(t, preserve)
	require.Equal(t, 2, calls)
	data, err := os.ReadFile(filepath.Join(dest, "intruder"))
	require.NoError(t, err)
	require.Equal(t, "intact", string(data))
	data, err = os.ReadFile(filepath.Join(root, "original", "original"))
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}

func TestRemoveScratchWithRestrictiveDirectories(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "scratch")
	restricted := filepath.Join(scratch, "tree", "restricted")
	require.NoError(t, os.MkdirAll(restricted, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(restricted, "data"), []byte("x"), 0o600))
	require.NoError(t, os.Chmod(restricted, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restricted, 0o700) })
	root, err := os.OpenRoot(scratch)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, removeScratch(root, scratch))
	_, err = os.Lstat(scratch)
	require.True(t, os.IsNotExist(err))
}

func TestExchangeDestinationRetainsDisplacedTreeWhenRollbackFails(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(scratch, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(scratch, "tree"), 0o700))
	stageInfo, err := os.Lstat(filepath.Join(scratch, "tree"))
	require.NoError(t, err)
	destInfo, err := os.Lstat(dest)
	require.NoError(t, err)
	stageParent, err := os.Open(scratch)
	require.NoError(t, err)
	defer stageParent.Close()
	destParent, err := os.Open(root)
	require.NoError(t, err)
	defer destParent.Close()
	calls := 0
	swap := func(a *os.File, b string, c *os.File, d string) error {
		calls++
		if calls == 1 {
			require.NoError(t, os.Rename(dest, filepath.Join(root, "original")))
			require.NoError(t, os.Mkdir(dest, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dest, "intruder"), []byte("keep"), 0o600))
			return exchangeDirectories(a, b, c, d)
		}
		return errors.New("rollback failed")
	}
	preserve, err := exchangeDestination(&exchangePair{stageParent: stageParent, stage: "tree", destParent: destParent, dest: "dest", stageInfo: stageInfo, destInfo: destInfo}, swap)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
	require.True(t, preserve)
	require.ErrorContains(t, err, scratch)
	data, err := os.ReadFile(filepath.Join(scratch, "tree", "intruder"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

func TestRenameExclusiveNeverOverwritesExistingDestination(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(stage, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.Error(t, renameExclusive(stage, dest))
	_, err := os.Stat(stage)
	require.NoError(t, err)
	_, err = os.Stat(dest)
	require.NoError(t, err)
}

func TestRemoveScratchDoesNotFollowSwappedPath(t *testing.T) {
	parent := t.TempDir()
	scratch := filepath.Join(parent, "scratch")
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(scratch, 0o700))
	require.NoError(t, os.Mkdir(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0o600))
	root, err := os.OpenRoot(scratch)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, os.Rename(scratch, filepath.Join(parent, "moved")))
	require.NoError(t, os.Symlink(outside, scratch))
	require.ErrorIs(t, removeScratch(root, scratch), errUtils.ErrArtifactUnsafePath)
	data, err := os.ReadFile(filepath.Join(outside, "keep"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

func TestExchangeDestinationRejectsSwappedStage(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(scratch, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(scratch, "tree"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "original"), []byte("keep"), 0o600))
	stageInfo, err := os.Lstat(filepath.Join(scratch, "tree"))
	require.NoError(t, err)
	destInfo, err := os.Lstat(dest)
	require.NoError(t, err)
	stageParent, err := os.Open(scratch)
	require.NoError(t, err)
	defer stageParent.Close()
	destParent, err := os.Open(root)
	require.NoError(t, err)
	defer destParent.Close()
	calls := 0
	swap := func(a *os.File, b string, c *os.File, d string) error {
		calls++
		if calls == 1 {
			require.NoError(t, os.Rename(filepath.Join(scratch, "tree"), filepath.Join(scratch, "real")))
			require.NoError(t, os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(scratch, "tree")))
		}
		return exchangeDirectories(a, b, c, d)
	}
	pair := &exchangePair{stageParent: stageParent, stage: "tree", destParent: destParent, dest: "dest", stageInfo: stageInfo, destInfo: destInfo}
	preserve, err := exchangeDestination(pair, swap)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
	require.True(t, preserve)
	require.Equal(t, 2, calls)
	data, err := os.ReadFile(filepath.Join(dest, "original"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	info, err := os.Lstat(filepath.Join(scratch, "tree"))
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink)
}
