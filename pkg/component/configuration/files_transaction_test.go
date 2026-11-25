package configuration

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRemovedFileIsNotRestoredAfterALaterConflict(t *testing.T) {
	root := t.TempDir()
	gone := schema.ComponentConfigurationFile{Path: "gone.tf", Content: []byte("gone"), IfExists: "overwrite"}
	other := schema.ComponentConfigurationFile{Path: "other.tf", Content: []byte("other"), IfExists: "error"}
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{gone}))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.tf"), []byte("authored"), 0o600))

	gone.IfExists = "remove"
	err := WriteFiles(root, []schema.ComponentConfigurationFile{gone, other})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoFileExists(t, filepath.Join(root, "gone.tf"))

	recorded := readManifest(mustRoot(t, root))
	assert.NotContains(t, recorded, "gone.tf", "a file this batch removed has no ownership record")
}

func TestConflictKeepsRecordsOfFilesTheBatchDidNotReach(t *testing.T) {
	root := t.TempDir()
	first := schema.ComponentConfigurationFile{Path: "first.tf", Content: []byte("first"), IfExists: "error"}
	second := schema.ComponentConfigurationFile{Path: "second.tf", Content: []byte("second"), IfExists: "error"}
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{first, second}))
	require.NoError(t, os.WriteFile(filepath.Join(root, "first.tf"), []byte("authored"), 0o600))

	require.Error(t, WriteFiles(root, []schema.ComponentConfigurationFile{first, second}))
	recorded := readManifest(mustRoot(t, root))
	assert.Contains(t, recorded, "first.tf", "the conflicting file keeps its earlier record")
	assert.Contains(t, recorded, "second.tf", "an unreached file keeps its earlier record")
}

func TestStaleFileRecordSurvivesAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict reads here")
	}
	root := t.TempDir()
	file := schema.ComponentConfigurationFile{Path: "stale.tf", Content: []byte("stale"), IfExists: "overwrite"}
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
	path := filepath.Join(root, "stale.tf")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	err := WriteFiles(root, nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.FileExists(t, path)
	assert.Contains(t, readManifest(mustRoot(t, root)), "stale.tf", "the record is kept for a later cleanup")

	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, WriteFiles(root, nil))
	require.NoFileExists(t, path, "restoring read access lets the retry remove the file")
}

func TestReadFileDigestDistinguishesAbsenceFromFailure(t *testing.T) {
	root := mustRoot(t, t.TempDir())
	_, exists, err := readFileDigest(root, "missing")
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, root.Mkdir("dir", 0o755))
	_, exists, err = readFileDigest(root, "dir")
	require.NoError(t, err)
	assert.False(t, exists, "a nonregular entry is not a file")
}

func TestWriteFilesSerializesWritersPerWorkdir(t *testing.T) {
	root := t.TempDir()
	var group sync.WaitGroup
	for index := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			file := schema.ComponentConfigurationFile{Path: "shared.tf", Content: []byte{byte('a' + index)}, IfExists: "overwrite"}
			assert.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
		}()
	}
	group.Wait()
	recorded := readManifest(mustRoot(t, root))
	content, err := os.ReadFile(filepath.Join(root, "shared.tf"))
	require.NoError(t, err)
	assert.Equal(t, contentDigest(content), recorded["shared.tf"], "the manifest matches the bytes that landed")
	lock, err := workdirLockPath(root)
	require.NoError(t, err)
	spelled, err := workdirLockPath(filepath.Join(root, ".", ""))
	require.NoError(t, err)
	assert.Equal(t, lock, spelled)
	// The lock lives in the working directory, so writers with different TMPDIR values share it.
	t.Setenv("TMPDIR", t.TempDir())
	elsewhere, err := workdirLockPath(root)
	require.NoError(t, err)
	assert.Equal(t, lock, elsewhere)
	assert.FileExists(t, lock)
}

func mustRoot(t *testing.T, directory string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// A batch with a handoff commits nothing when a later file conflicts: the files it wrote and the
// manifest are rolled back, while the same failure without a handoff keeps the partial progress.
func TestHandoffBatchRollsBackWrittenFilesOnConflict(t *testing.T) {
	files := []schema.ComponentConfigurationFile{
		{Path: "remote.tf", Content: []byte("# generated"), IfExists: "overwrite"},
		{Path: "other.tf", Content: []byte("# generated"), IfExists: "error"},
	}
	setup := func(t *testing.T) string {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "other.tf"), []byte("authored"), 0o600))
		return root
	}

	t.Run("with a handoff", func(t *testing.T) {
		root := setup(t)
		err := WriteFilesWithHandoff(root, files, &Handoff{Run: func() error { return nil }})
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		assert.NoFileExists(t, filepath.Join(root, "remote.tf"), "the written file is rolled back")
		assert.NotContains(t, readManifest(mustRoot(t, root)), "remote.tf", "its digest is not committed")
		authored, readErr := os.ReadFile(filepath.Join(root, "other.tf"))
		require.NoError(t, readErr)
		assert.Equal(t, "authored", string(authored))
	})

	t.Run("without a handoff", func(t *testing.T) {
		root := setup(t)
		require.ErrorIs(t, WriteFiles(root, files), errUtils.ErrPluginInvalidConfig)
		assert.FileExists(t, filepath.Join(root, "remote.tf"), "partial progress is kept")
		assert.Contains(t, readManifest(mustRoot(t, root)), "remote.tf")
	})
}

// The handoff's validation runs while the workdir lock is held, so another writer cannot change the
// checked files between the check and the batch; a validation failure writes nothing.
func TestHandoffValidateRunsUnderWorkdirLock(t *testing.T) {
	root := t.TempDir()
	var entered atomic.Bool
	done := make(chan struct{})
	files := []schema.ComponentConfigurationFile{{Path: "a.tf", Content: []byte("a"), IfExists: "overwrite"}}
	err := WriteFilesWithHandoff(root, files, &Handoff{Validate: func() error {
		go func() {
			defer close(done)
			assert.NoError(t, WithWorkdirLock(root, func() error { entered.Store(true); return nil }))
		}()
		time.Sleep(100 * time.Millisecond)
		assert.False(t, entered.Load(), "another writer waits while validation runs")
		return nil
	}})
	require.NoError(t, err)
	<-done
	assert.True(t, entered.Load(), "the lock is released after the batch")

	failure := errUtils.ErrPluginInvalidConfig
	err = WriteFilesWithHandoff(root, []schema.ComponentConfigurationFile{{Path: "b.tf", Content: []byte("b"), IfExists: "overwrite"}},
		&Handoff{Validate: func() error { return failure }})
	require.ErrorIs(t, err, failure)
	assert.NoFileExists(t, filepath.Join(root, "b.tf"))
}
