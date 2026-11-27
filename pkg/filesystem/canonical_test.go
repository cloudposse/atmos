package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalPath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, suffix := range []string{"", "new/dir/file"} {
		result, err := CanonicalPath(filepath.Join(alias, suffix))
		require.NoError(t, err)
		require.Equal(t, filepath.Join(root, suffix), result)
	}
	dangling := filepath.Join(root, "dangling")
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), dangling))
	_, err = CanonicalPath(filepath.Join(dangling, "state"))
	require.Error(t, err)
}
