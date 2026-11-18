package oci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
)

// corruptBlob overwrites a layout blob with other bytes of the same length. Only layouts are
// corrupted on disk; the helper skips registry repositories, whose blobs are not files.
func corruptBlob(t *testing.T, repo *Repository, dgst digest.Digest) {
	t.Helper()
	layout, ok := repo.be.(*layoutBackend)
	if !ok {
		t.Skip("blob corruption is exercised on layouts, where blobs are files")
	}
	path := filepath.Join(layout.root, "blobs", dgst.Algorithm().String(), dgst.Encoded())
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xff
	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

// writeSource writes a tiny package source tree.
func writeSource(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("service source"), 0o644))
}
