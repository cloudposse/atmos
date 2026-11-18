package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// Replacing an object keeps the mode of the file on disk, in both layouts, and never exposes the
// new content under a wider mode.
func TestPutReplaceKeepsDestinationMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX permission bits are not enforced on %s", runtime.GOOS)
	}

	layouts := []struct {
		name string
		opts []Option
	}{
		{name: "managed"},
		{name: "plain files", opts: []Option{WithPlainFileCompatibility()}},
	}
	modes := []os.FileMode{0o600, 0o644, 0o664}
	for _, layout := range layouts {
		for _, mode := range modes {
			t.Run(layout.name+"/"+mode.String(), func(t *testing.T) {
				s := newStore(t, layout.opts...)
				require.NoError(t, putBody(t, s, "dir/obj", "old", blob.Condition{}))
				path := filepath.Join(s.root, "dir", "obj")
				require.NoError(t, os.Chmod(path, mode))

				require.NoError(t, putBody(t, s, "dir/obj", "new contents", blob.Condition{}))

				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, mode, info.Mode().Perm())
				assert.Equal(t, "new contents", readBody(t, s, "dir/obj"))
			})
		}
	}
}

// A destination that does not exist yet gets the store's default mode, and a failed put leaves the
// existing destination untouched.
func TestPutNewObjectUsesStoreMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX permission bits are not enforced on %s", runtime.GOOS)
	}
	s := newStore(t, WithPlainFileCompatibility())
	require.NoError(t, putBody(t, s, "fresh", "data", blob.Condition{}))

	info, err := os.Stat(filepath.Join(s.root, "fresh"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&^DefaultFilePerm)

	require.NoError(t, os.Chmod(filepath.Join(s.root, "fresh"), 0o600))
	_, err = s.Put(context.Background(), "fresh", iotest.ErrReader(os.ErrDeadlineExceeded), blob.PutOptions{Size: blob.UnknownSize})
	require.Error(t, err)
	info, err = os.Stat(filepath.Join(s.root, "fresh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, "data", readBody(t, s, "fresh"))
}
