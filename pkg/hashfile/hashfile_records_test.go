package hashfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHashFilesAndRecords_NilRecordsMatchesHashFiles asserts that a nil (or empty) records slice
// produces a digest byte-identical to HashFiles(paths) -- HashFiles is implemented in terms of
// HashFilesAndRecords specifically to guarantee this, and any drift here would be a regression in
// that guarantee, breaking every existing caller of HashFiles.
func TestHashFilesAndRecords_NilRecordsMatchesHashFiles(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	b := filepath.Join(tmpDir, "b.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("world"), 0o644))

	paths := []string{a, b}

	want, err := HashFiles(paths)
	require.NoError(t, err)

	gotNil, err := HashFilesAndRecords(paths, nil)
	require.NoError(t, err)
	assert.Equal(t, want, gotNil, "nil records must produce the same digest as HashFiles")

	gotEmpty, err := HashFilesAndRecords(paths, []string{})
	require.NoError(t, err)
	assert.Equal(t, want, gotEmpty, "empty records must produce the same digest as HashFiles")
}

// TestHashFilesAndRecords_Deterministic asserts the digest is stable across repeated calls with
// the same inputs.
func TestHashFilesAndRecords_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))

	records := []string{"k1=v1", "k2=v2"}

	h1, err := HashFilesAndRecords([]string{a}, records)
	require.NoError(t, err)
	h2, err := HashFilesAndRecords([]string{a}, records)
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
	assert.NotEmpty(t, h1)
}

// TestHashFilesAndRecords_RecordOrderIndependent asserts that supplying the same records in a
// different order produces the same digest -- callers (e.g. a fingerprint built from an unordered
// map) must not have to pre-sort records themselves.
func TestHashFilesAndRecords_RecordOrderIndependent(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))

	h1, err := HashFilesAndRecords([]string{a}, []string{"a=1", "b=2", "c=3"})
	require.NoError(t, err)
	h2, err := HashFilesAndRecords([]string{a}, []string{"c=3", "a=1", "b=2"})
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "record order must not affect the digest")
}

// TestHashFilesAndRecords_RecordChangeChangesHash asserts that changing a record value (with
// files held constant) changes the digest -- records must actually participate in the hash, not
// be silently ignored.
func TestHashFilesAndRecords_RecordChangeChangesHash(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "a.txt")
	require.NoError(t, os.WriteFile(a, []byte("hello"), 0o644))

	h1, err := HashFilesAndRecords([]string{a}, []string{"key=v1"})
	require.NoError(t, err)
	h2, err := HashFilesAndRecords([]string{a}, []string{"key=v2"})
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "changing a record's value must change the digest")

	h3, err := HashFilesAndRecords([]string{a}, nil)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h3, "adding a record where none existed must change the digest")
}

// TestHashFilesAndRecords_PropagatesRecordWriteError asserts a write failure while hashing the
// records tail is surfaced, not swallowed -- forces the writeSortedRecords error branch inside
// HashFilesAndRecords (reached only after paths, which are empty here, have hashed successfully).
func TestHashFilesAndRecords_PropagatesRecordWriteError(t *testing.T) {
	// No files, so the only way to reach a write error is through the records tail; verify via
	// writeSortedRecords directly since HashFilesAndRecords always writes to a real sha256.Hash
	// (which never errors) and therefore cannot exercise this branch through its public path.
	wantErr := assert.AnError
	w := &erroringWriter{failOn: 1, err: wantErr}

	err := writeSortedRecords(w, []string{"a=1"})

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestHashNamedFiles_NamesIndependentOfPath asserts that two files at different absolute paths,
// hashed under the same logical name, produce the same digest -- this is the whole point of
// HashNamedFiles over HashFiles: chdir-independent fingerprints for e.g. two checkouts of the same
// component at different locations on disk.
func TestHashNamedFiles_NamesIndependentOfPath(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	path1 := filepath.Join(dir1, "main.tf")
	path2 := filepath.Join(dir2, "main.tf")
	require.NoError(t, os.WriteFile(path1, []byte("content"), 0o644))
	require.NoError(t, os.WriteFile(path2, []byte("content"), 0o644))

	h1, err := HashNamedFiles(map[string]string{"main.tf": path1}, nil)
	require.NoError(t, err)
	h2, err := HashNamedFiles(map[string]string{"main.tf": path2}, nil)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "identical content under the same logical name must hash identically regardless of absolute path")
}

// TestHashNamedFiles_NameChangeChangesHash asserts that hashing the same file content under a
// different logical name changes the digest -- names, not just content, must participate.
func TestHashNamedFiles_NameChangeChangesHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.tf")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))

	h1, err := HashNamedFiles(map[string]string{"main.tf": path}, nil)
	require.NoError(t, err)
	h2, err := HashNamedFiles(map[string]string{"other.tf": path}, nil)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "a different logical name must change the digest even with identical content")
}

// TestHashNamedFiles_MapIterationOrderIndependent asserts the digest doesn't depend on Go's
// randomized map iteration order -- names are sorted internally before hashing.
func TestHashNamedFiles_MapIterationOrderIndependent(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.tf")
	pathB := filepath.Join(dir, "b.tf")
	require.NoError(t, os.WriteFile(pathA, []byte("a-content"), 0o644))
	require.NoError(t, os.WriteFile(pathB, []byte("b-content"), 0o644))

	named := map[string]string{"a.tf": pathA, "b.tf": pathB}

	var last string
	for i := 0; i < 5; i++ {
		h, err := HashNamedFiles(named, []string{"k=v"})
		require.NoError(t, err)
		if i > 0 {
			assert.Equal(t, last, h)
		}
		last = h
	}
}

// TestHashNamedFiles_MissingFileErrors asserts a missing named file surfaces an error rather than
// being silently skipped -- silently dropping a file from the digest would make the fingerprint
// blind to that file's absence.
func TestHashNamedFiles_MissingFileErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := HashNamedFiles(map[string]string{"missing.tf": filepath.Join(dir, "missing.tf")}, nil)

	require.Error(t, err)
}
