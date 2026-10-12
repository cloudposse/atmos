package toolchain

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfChmodUnenforced skips tests that rely on directory permissions being enforced.
func skipIfChmodUnenforced(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not restrict directory writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod does not restrict file creation")
	}
}

func TestLoadToolVersions_ReadOnlyDirectoryFallsBackToUnlockedRead(t *testing.T) {
	skipIfChmodUnenforced(t)

	dir := t.TempDir()
	manifest := filepath.Join(dir, ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.9.8\nhelm 3.16.0\n"), 0o644))

	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	// Prerequisite: the lock file really cannot be created in this directory.
	_, err := os.OpenFile(manifest+".lock", os.O_CREATE|os.O_RDONLY, 0o644)
	require.Error(t, err, "prerequisite: lock file creation must be denied")
	require.ErrorIs(t, err, fs.ErrPermission)

	versions, err := LoadToolVersions(manifest)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.9.8"}, versions.Tools["terraform"])
	assert.Equal(t, []string{"3.16.0"}, versions.Tools["helm"])

	_, statErr := os.Stat(manifest + ".lock")
	assert.True(t, errors.Is(statErr, fs.ErrNotExist), "no lock file should have been created, got %v", statErr)
}

func TestSaveToolVersions_ReadOnlyDirectoryStillRequiresLock(t *testing.T) {
	skipIfChmodUnenforced(t)

	dir := t.TempDir()
	manifest := filepath.Join(dir, ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.9.8\n"), 0o644))

	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := SaveToolVersions(manifest, &ToolVersions{Tools: map[string][]string{"terraform": {"1.10.0"}}})
	require.Error(t, err, "writes must keep requiring the lock")

	data, readErr := os.ReadFile(manifest)
	require.NoError(t, readErr)
	assert.Equal(t, "terraform 1.9.8\n", string(data))
}

func TestWithToolVersionsSharedLock_DoesNotSwallowCallbackErrors(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.9.8\n"), 0o644))

	calls := 0
	err := withToolVersionsSharedLock(manifest, func() error {
		calls++
		return fs.ErrPermission
	})

	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Equal(t, 1, calls, "callback error must not trigger an unlocked retry")
}

func TestIsLockUnavailableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "permission", err: &fs.PathError{Op: "open", Path: "x.lock", Err: fs.ErrPermission}, want: true},
		{name: "not exist is not a fallback case", err: fs.ErrNotExist, want: false},
		{name: "arbitrary error", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLockUnavailableError(tt.err))
		})
	}
}

func TestIsReadOnlyFilesystemError(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.False(t, isReadOnlyFilesystemError(errors.New("anything")))
		return
	}
	wrapped := &fs.PathError{Op: "open", Path: "x.lock", Err: syscall.EROFS}
	assert.True(t, isReadOnlyFilesystemError(wrapped))
	assert.True(t, isLockUnavailableError(wrapped))
	assert.False(t, isReadOnlyFilesystemError(errors.New("anything")))
}

// TestLoadToolVersionsLenient verifies that the lenient loader skips lines without
// a version while the strict loader keeps rejecting them, so callers that rewrite
// the file never drop a line.
func TestLoadToolVersionsLenient(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tool-versions")
	require.NoError(t, os.WriteFile(path, []byte("terraform\n# comment\njqlang/jq 1.7.1 1.6\n"), 0o600))

	lenient, err := LoadToolVersionsLenient(path)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"jqlang/jq": {"1.7.1", "1.6"}}, lenient.Tools)

	_, err = LoadToolVersions(path)
	require.ErrorIs(t, err, ErrInvalidToolSpec)

	_, err = LoadToolVersionsLenient(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
