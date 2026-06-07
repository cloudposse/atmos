package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRoot_RoundTrip(t *testing.T) {
	src := t.TempDir()
	// Build a nested tree. Keep first/last entries deterministic by name.
	files := map[string]string{
		"a-first.txt": "first",
		filepath.Join("toolchain", "bin", "tool"): "binary",
		filepath.Join("toolchain", "lock.yaml"):   "lock",
		"z-last.txt":                              "last",
	}
	for rel, content := range files {
		full := filepath.Join(src, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	// State dir must be excluded from the archive.
	require.NoError(t, os.MkdirAll(filepath.Join(src, stateDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, stateDirName, "state.json"), []byte("{}"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, nil))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	// Assert contents by value (first and last entries explicitly).
	gotFirst, err := os.ReadFile(filepath.Join(dst, "a-first.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first", string(gotFirst))

	gotLast, err := os.ReadFile(filepath.Join(dst, "z-last.txt"))
	require.NoError(t, err)
	assert.Equal(t, "last", string(gotLast))

	gotTool, err := os.ReadFile(filepath.Join(dst, "toolchain", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "binary", string(gotTool))

	// State dir must NOT have been archived/restored.
	_, statErr := os.Stat(filepath.Join(dst, stateDirName))
	assert.True(t, os.IsNotExist(statErr), "state dir must be excluded from the archive")
}

func TestArchiveRoot_IncludesFilter(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "toolchain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "toolchain", "tool"), []byte("keep"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "other"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "other", "skip"), []byte("drop"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, []string{"toolchain"}))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	_, err := os.Stat(filepath.Join(dst, "toolchain", "tool"))
	assert.NoError(t, err, "included path must be archived")

	_, err = os.Stat(filepath.Join(dst, "other", "skip"))
	assert.True(t, os.IsNotExist(err), "excluded path must not be archived")
}

func TestSafeJoin_RejectsEscape(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "root")

	// Hostile entry names that attempt to escape the root must be rejected.
	for _, name := range []string{
		"../escape",
		filepath.Join("..", "..", "etc", "passwd"),
		filepath.Join("sub", "..", "..", "escape"),
	} {
		_, err := safeJoin(root, name)
		require.Error(t, err, "entry %q must be rejected", name)
	}

	// Legitimate nested entries resolve within the root.
	ok, err := safeJoin(root, filepath.Join("sub", "file"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "sub", "file"), ok)

	okNested, err := safeJoin(root, filepath.Join("a", "b", "c.txt"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "a", "b", "c.txt"), okNested)
}
