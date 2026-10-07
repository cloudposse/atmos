package target

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalPublishFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "a.zip"), []byte{0, 255, 1}, 0o600))
	files, err := LocalPublishFiles(root, "release/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "release/sub/a.zip", files[0].Name)
	for range 2 {
		reader, err := files[0].Open()
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		assert.Equal(t, []byte{0, 255, 1}, data)
	}
	file := filepath.Join(root, "sub", "a.zip")
	for _, invalid := range []string{".", "./", "prefix/./artifact", "../escape", "/absolute", "C:/windows", "back\\slash", ".git/config"} {
		_, err := LocalPublishFiles(file, invalid)
		require.Error(t, err, invalid)
	}
	files, err = LocalPublishFiles(file, "renamed.zip")
	require.NoError(t, err)
	assert.Equal(t, "renamed.zip", files[0].Name)
	require.NoError(t, os.WriteFile(file, []byte("changed"), 0o600))
	_, err = files[0].Open()
	require.Error(t, err, "detect replaced or resized source")
}

func TestLocalPublishRejectsSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "source")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))
	link := filepath.Join(root, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := LocalPublishFiles(root, "")
	require.Error(t, err)
	_, err = LocalPublishFiles(link, "")
	require.Error(t, err)
}

func TestPublishNeverFallsBackToDeployment(t *testing.T) {
	t.Parallel()
	provisioner := &fakeProvisioner{}
	Register("publish-test-deployment-only", provisioner)
	_, err := Publisher("publish-test-deployment-only")
	require.Error(t, err)
	assert.Zero(t, provisioner.delivered)
	_, err = Publisher("publish-test-unknown")
	require.Error(t, err)
}
