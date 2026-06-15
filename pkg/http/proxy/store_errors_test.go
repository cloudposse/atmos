package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestFileStore_CommitMkdirError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires enforced non-root POSIX directory permissions")
	}
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	store := NewFileStore(root)
	_, err := store.Commit(t.Context(), CommitRequest{
		Key:  "providers/example.com/ns/name/obj.zip",
		Data: bytes.NewReader([]byte("data")),
		Kind: KindArtifact,
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
}

func TestFileStore_CommitWriteTempError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires enforced non-root POSIX directory permissions")
	}
	root := t.TempDir()
	store := NewFileStore(root)

	// Pre-create the object's parent dir read-only so Commit's MkdirAll succeeds (the
	// dir already exists) but the temp-object create inside it fails.
	objDir := filepath.Dir(filepath.Join(root, "providers", "example.com", "ns", "name", "obj.zip"))
	require.NoError(t, os.MkdirAll(objDir, 0o755))
	require.NoError(t, os.Chmod(objDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(objDir, 0o700) })

	_, err := store.Commit(t.Context(), CommitRequest{
		Key:  "providers/example.com/ns/name/obj.zip",
		Data: bytes.NewReader([]byte("data")),
		Kind: KindArtifact,
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
}
