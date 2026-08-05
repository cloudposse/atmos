package hashfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashFiles_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	b := filepath.Join(tmpDir, "b.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("world"), 0o644))

	h1, err := HashFiles([]string{a, b})
	require.NoError(t, err)
	h2, err := HashFiles([]string{b, a}) // reversed order.
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "hash must be stable regardless of input path order")
	assert.NotEmpty(t, h1)
}

func TestHashFiles_ContentChangeChangesHash(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))

	before, err := HashFiles([]string{a})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(a, []byte("hello!"), 0o644))
	after, err := HashFiles([]string{a})
	require.NoError(t, err)

	assert.NotEqual(t, before, after)
}

func TestHashFiles_RenameChangesHash(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	renamed := filepath.Join(tmpDir, "renamed.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))

	before, err := HashFiles([]string{a})
	require.NoError(t, err)

	require.NoError(t, os.Rename(a, renamed))
	after, err := HashFiles([]string{renamed})
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "a rename must change the hash even though content is unchanged")
}

func TestHashFiles_EmptyInput(t *testing.T) {
	h, err := HashFiles(nil)
	require.NoError(t, err)
	assert.NotEmpty(t, h, "empty input still produces a stable (sha256 of nothing) hash")
}

func TestHashFiles_MissingFileErrors(t *testing.T) {
	_, err := HashFiles([]string{"/no/such/file"})
	require.Error(t, err)
}
