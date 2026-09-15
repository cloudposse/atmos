package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMaterializeRestoresTargetOnCopyFailure(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "original")
	require.NoError(t, record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.MkdirAll(target, 0o755))
		return os.WriteFile(filepath.Join(target, "main.tf"), []byte("original"), 0o644)
	}))
	require.NoError(t, os.WriteFile(filepath.Join(target, "unowned.txt"), []byte("keep"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(target, "empty"), 0o750))
	before, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	copyErr := errors.New("partial copy failed")
	err = record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("changed"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "new.tf"), []byte("partial"), 0o644))
		return copyErr
	})
	require.ErrorIs(t, err, copyErr)
	assertFileContents(t, filepath.Join(target, "main.tf"), "original")
	assertFileContents(t, filepath.Join(target, "unowned.txt"), "keep")
	assert.NoFileExists(t, filepath.Join(target, "new.tf"))
	assert.DirExists(t, filepath.Join(target, "empty"))
	assertFileContents(t, Path(config), string(before))
}

func TestMaterializeRestoresCopiedAndPrunedFilesOnReceiptFailure(t *testing.T) {
	skipUnlessWritablePermissionsWork(t)
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Vendor.LockFile = filepath.Join(config.BasePath, "receipts", "vendor.lock.yaml")
	record, target := transactionRecord(t, config, "original")
	stale := filepath.Join(target, "stale.tf")
	record.artifact.Files = append(record.artifact.Files, File{Path: "stale.tf", Type: "file", Mode: 0o644, SHA256: hashString("stale")})
	require.NoError(t, record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o644))
		return os.WriteFile(filepath.Join(target, "main.tf"), []byte("original"), 0o644)
	}))
	before, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	record.artifact.Files = record.artifact.Files[:1]
	lockDir := filepath.Dir(Path(config))
	t.Cleanup(func() { require.NoError(t, os.Chmod(lockDir, 0o755)) })
	err = record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("changed"), 0o644))
		return os.Chmod(lockDir, 0o555)
	})
	require.ErrorIs(t, err, ErrCreateTempVendorLock)
	assertFileContents(t, filepath.Join(target, "main.tf"), "original")
	assertFileContents(t, stale, "stale")
	assertFileContents(t, Path(config), string(before))
}

func TestMaterializeRemovesFailedFirstInstallation(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, _ := transactionRecord(t, config, "first")
	target := filepath.Join(config.BasePath, "components", "terraform", "first")
	record.artifact.Target = target
	copyErr := errors.New("copy failed after creating target")
	err := record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("partial"), 0o644))
		return copyErr
	})
	require.ErrorIs(t, err, copyErr)
	assert.NoDirExists(t, filepath.Join(config.BasePath, "components"))
	assert.NoFileExists(t, Path(config))
}

func TestSnapshotCollapsesOverlappingRootsAndCleansBackup(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "first")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("original"), 0o644))
	snapshot, err := snapshotTargets(config, New(), record)
	require.NoError(t, err)
	t.Cleanup(func() { _ = snapshot.close() })
	require.Len(t, snapshot.entries, 1)
	require.DirExists(t, snapshot.dir)
	require.NoError(t, snapshot.close())
	assert.NoDirExists(t, snapshot.dir)
}

// assertFileContents checks both file presence and byte-for-byte restoration.
func assertFileContents(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}

func TestMaterializeRejectsTargetsContainingMutationState(t *testing.T) {
	for _, targetName := range []string{"receipts", ".atmos"} {
		t.Run(targetName, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			config.Vendor.LockFile = filepath.Join("receipts", "vendor.lock.yaml")
			record, _ := transactionRecord(t, config, "new")
			record.artifact.Target = filepath.Join(config.BasePath, targetName)
			called := false
			err := record.Materialize(context.Background(), config, func() error { called = true; return nil })
			require.ErrorIs(t, err, errRollbackOverlapsState)
			assert.False(t, called)
			assert.FileExists(t, Path(config)+".lock")
			assert.FileExists(t, filepath.Join(config.BasePath, ".atmos", "vendor-mutation.lock"))
		})
	}
}

func TestMaterializeRejectsUnsafeSymlinkDestinations(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(fmt.Sprint("dangling=", dangling), func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			record, target := transactionRecord(t, config, "new")
			require.NoError(t, os.MkdirAll(target, 0o755))
			outside := filepath.Join(t.TempDir(), "external.tf")
			if !dangling {
				require.NoError(t, os.WriteFile(outside, []byte("untouched"), 0o644))
			}
			require.NoError(t, os.Symlink(outside, filepath.Join(target, "main.tf")))
			called := false
			err := record.Materialize(context.Background(), config, func() error { called = true; return nil })
			require.Error(t, err)
			assert.False(t, called)
			if !dangling {
				assertFileContents(t, outside, "untouched")
			}
		})
	}
}

func TestMaterializeRestoresInternalSymlinkDestination(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "new")
	require.NoError(t, os.MkdirAll(target, 0o755))
	referent := filepath.Join(config.BasePath, "shared.tf")
	require.NoError(t, os.WriteFile(referent, []byte("original"), 0o600))
	link := filepath.Join(target, "main.tf")
	require.NoError(t, os.Symlink(referent, link))
	copyErr := errors.New("copy failed")
	err := record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.WriteFile(link, []byte("changed"), 0o600))
		return copyErr
	})
	require.ErrorIs(t, err, copyErr)
	assertFileContents(t, referent, "original")
	actual, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, referent, actual)
}

func TestMaterializeRestoresWithinReadOnlyParent(t *testing.T) {
	skipUnlessWritablePermissionsWork(t)
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, _ := transactionRecord(t, config, "new")
	parent := filepath.Join(config.BasePath, "readonly")
	target := filepath.Join(parent, "target")
	record.artifact.Target = target
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("original"), 0o644))
	require.NoError(t, os.Chmod(parent, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o755)) })
	copyErr := errors.New("partial copy")
	err := record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("changed"), 0o644))
		return copyErr
	})
	require.ErrorIs(t, err, copyErr)
	assertFileContents(t, filepath.Join(target, "main.tf"), "original")
}

func TestSnapshotPreservesReadOnlyDirectoriesAndCleansBackup(t *testing.T) {
	skipUnlessWritablePermissionsWork(t)
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "new")
	readonly := filepath.Join(target, "readonly")
	require.NoError(t, os.MkdirAll(readonly, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(readonly, "keep.tf"), []byte("keep"), 0o444))
	require.NoError(t, os.Chmod(readonly, 0o555))
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o755) })
	snapshot, err := snapshotTargets(config, New(), record)
	require.NoError(t, err)
	t.Cleanup(func() { _ = snapshot.close() })
	require.NoError(t, snapshot.restore())
	info, err := os.Stat(readonly)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o555), info.Mode().Perm())
	assertFileContents(t, filepath.Join(readonly, "keep.tf"), "keep")
	require.NoError(t, snapshot.close())
	assert.NoDirExists(t, snapshot.dir)
}

func TestCollapseRollbackRootsWithInterleavedSibling(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a")
	sibling := filepath.Join(base, "a-other")
	assert.Equal(t, []string{a, sibling}, collapseRollbackRoots([]string{sibling, filepath.Join(a, "child"), a}))
}

func TestMaterializeRejectsSymlinkedMutationState(t *testing.T) {
	for _, state := range []string{"project", "receipt"} {
		t.Run(state, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			record, target := transactionRecord(t, config, "new")
			require.NoError(t, os.MkdirAll(target, 0o755))
			held := filepath.Join(target, "held.lock")
			require.NoError(t, os.WriteFile(held, nil, 0o600))
			if state == "project" {
				require.NoError(t, os.Symlink(target, filepath.Join(config.BasePath, ".atmos")))
			} else {
				require.NoError(t, os.Symlink(held, Path(config)+".lock"))
			}
			called := false
			err := record.Materialize(context.Background(), config, func() error { called = true; return nil })
			require.ErrorIs(t, err, errRollbackOverlapsState)
			assert.False(t, called)
		})
	}
}

func TestMaterializeRestoresHardlinkAliases(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "new")
	require.NoError(t, os.MkdirAll(target, 0o755))
	main := filepath.Join(target, "main.tf")
	alias := filepath.Join(config.BasePath, "shared.tf")
	require.NoError(t, os.WriteFile(main, []byte("original"), 0o644))
	require.NoError(t, os.Link(main, alias))
	copyErr := errors.New("partial copy")
	err := record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.WriteFile(main, []byte("changed"), 0o644))
		return copyErr
	})
	require.ErrorIs(t, err, copyErr)
	assertFileContents(t, main, "original")
	assertFileContents(t, alias, "original")
	mainInfo, err := os.Stat(main)
	require.NoError(t, err)
	aliasInfo, err := os.Stat(alias)
	require.NoError(t, err)
	assert.True(t, os.SameFile(mainInfo, aliasInfo))
}
