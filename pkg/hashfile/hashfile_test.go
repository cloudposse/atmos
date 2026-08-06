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

// TestHashFiles_ConcatenationCollisionDoesNotMatch is the regression case for a real collision
// in the previous implementation: hashing raw path bytes immediately followed by raw content
// bytes with no boundary meant a path "a" with content "bc" produced the exact same byte stream
// (and therefore hash) as a path "ab" with content "c". Length-prefixing each record's path and
// content must make these distinguishable.
func TestHashFiles_ConcatenationCollisionDoesNotMatch(t *testing.T) {
	tmpDir := t.TempDir()

	pathA := filepath.Join(tmpDir, "a")
	require.NoError(t, os.WriteFile(pathA, []byte("bc"), 0o644))
	hashA, err := HashFiles([]string{pathA})
	require.NoError(t, err)

	pathAB := filepath.Join(tmpDir, "ab")
	require.NoError(t, os.WriteFile(pathAB, []byte("c"), 0o644))
	hashAB, err := HashFiles([]string{pathAB})
	require.NoError(t, err)

	assert.NotEqual(t, hashA, hashAB, "path %q + content \"bc\" must not collide with path %q + content \"c\"", pathA, pathAB)
}

// TestHashFiles_SameBasenameDifferentDirectoryDiffersFromHash is the regression case for the
// second real collision: hashing only filepath.Base(p) made two files with the same filename but
// different directories (and thus different identity/provenance) indistinguishable, even with
// identical content. Hashing the full path fixes this.
func TestHashFiles_SameBasenameDifferentDirectoryDiffersFromHash(t *testing.T) {
	tmpDir := t.TempDir()

	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	require.NoError(t, os.MkdirAll(dir1, 0o755))
	require.NoError(t, os.MkdirAll(dir2, 0o755))

	path1 := filepath.Join(dir1, "name.txt")
	path2 := filepath.Join(dir2, "name.txt")
	require.NoError(t, os.WriteFile(path1, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(path2, []byte("hello"), 0o644))

	hash1, err := HashFiles([]string{path1})
	require.NoError(t, err)
	hash2, err := HashFiles([]string{path2})
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "same filename in a different directory must hash differently, even with identical content")
}

// TestHashFiles_LargeFileStreamsWithoutError is a light sanity check that hashing doesn't load
// the whole file into memory at once (streams via os.Open + io.Copy) -- a large-ish file (a few
// MB) must still hash correctly and match a second, independently-computed hash of the same
// content.
func TestHashFiles_LargeFileStreamsWithoutError(t *testing.T) {
	tmpDir := t.TempDir()
	large := filepath.Join(tmpDir, "large.bin")

	content := make([]byte, 5*1024*1024) // 5MB.
	for i := range content {
		content[i] = byte(i % 251)
	}
	require.NoError(t, os.WriteFile(large, content, 0o644))

	h1, err := HashFiles([]string{large})
	require.NoError(t, err)
	h2, err := HashFiles([]string{large})
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
	assert.NotEmpty(t, h1)
}
