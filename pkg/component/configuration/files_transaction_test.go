package configuration

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

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
	assert.Equal(t, workdirLockPath(root), workdirLockPath(filepath.Join(root, ".", "")))
}

func mustRoot(t *testing.T, directory string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	return root
}
