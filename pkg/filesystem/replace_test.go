package filesystem

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
	"go.uber.org/mock/gomock"
)

// renameFailingFileSystem returns a mock whose atomic write fails with atomicErr while every other
// call used here goes to the real OS, so a test can simulate a rename the kernel refuses (for
// example EBUSY on a single-file bind mount) on any platform.
func renameFailingFileSystem(t *testing.T, atomicErr error) *MockFileSystem {
	t.Helper()
	mock := NewMockFileSystem(gomock.NewController(t))
	osFS := NewOSFileSystem()
	mock.EXPECT().WriteFileAtomic(gomock.Any(), gomock.Any(), gomock.Any()).Return(atomicErr).AnyTimes()
	mock.EXPECT().Stat(gomock.Any()).DoAndReturn(osFS.Stat).AnyTimes()
	mock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(osFS.WriteFile).AnyTimes()
	return mock
}

// TestWriteFileAtomicOrInPlace_FallsBackWhenReplaceIsRefused reproduces the 1.235.0 upgrade
// regression for files Atmos shares with other tools: when the temp file + rename replace is
// refused (a single-file bind mount answers EBUSY, a rename across devices EXDEV, a read-only
// directory EACCES/EPERM/EROFS), the same bytes are written in place, as v1.234.1 did.
func TestWriteFileAtomicOrInPlace_FallsBackWhenReplaceIsRefused(t *testing.T) {
	for name, errno := range map[string]syscall.Errno{
		"EBUSY": syscall.EBUSY, "EXDEV": syscall.EXDEV, "EACCES": syscall.EACCES,
		"EPERM": syscall.EPERM, "EROFS": syscall.EROFS,
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(target, []byte("old"), 0o640))
			atomicErr := &os.LinkError{Op: "rename", Old: target + ".tmp", New: target, Err: errno}

			require.NoError(t, WriteFileAtomicOrInPlace(renameFailingFileSystem(t, atomicErr), target, []byte("new"), 0o600))

			got, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, "new", string(got))
			assertSamePerm(t, target, 0o640)
		})
	}
}

// TestWriteFileAtomicOrInPlace_NoFallback covers the cases that must not fall back: an atomic
// write that succeeds, an unrelated failure, and a target that does not exist yet (an in-place
// write cannot help when there is nothing to write into).
func TestWriteFileAtomicOrInPlace_NoFallback(t *testing.T) {
	t.Run("atomic write succeeds", func(t *testing.T) {
		mock := NewMockFileSystem(gomock.NewController(t))
		mock.EXPECT().WriteFileAtomic("target", []byte("data"), os.FileMode(0o600)).Return(nil)
		require.NoError(t, WriteFileAtomicOrInPlace(mock, "target", []byte("data"), 0o600))
	})

	t.Run("unrelated failure", func(t *testing.T) {
		mock := NewMockFileSystem(gomock.NewController(t))
		failure := errors.New("disk full")
		mock.EXPECT().WriteFileAtomic(gomock.Any(), gomock.Any(), gomock.Any()).Return(failure)
		err := WriteFileAtomicOrInPlace(mock, "target", []byte("data"), 0o600)
		require.ErrorIs(t, err, failure)
	})

	t.Run("missing target", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "absent.json")
		atomicErr := &os.PathError{Op: "open", Path: target, Err: syscall.EACCES}
		err := WriteFileAtomicOrInPlace(renameFailingFileSystem(t, atomicErr), target, []byte("data"), 0o600)
		require.ErrorIs(t, err, fs.ErrPermission)
		_, statErr := os.Stat(target)
		assert.True(t, os.IsNotExist(statErr), "no file is created by the fallback")
	})

	t.Run("target is a directory", func(t *testing.T) {
		target := t.TempDir()
		atomicErr := &os.LinkError{Op: "rename", Err: syscall.EBUSY}
		err := WriteFileAtomicOrInPlace(renameFailingFileSystem(t, atomicErr), target, []byte("data"), 0o600)
		require.ErrorIs(t, err, syscall.EBUSY)
	})
}

// TestWriteFileAtomicOrInPlace_InPlaceFailureReportsBoth verifies that when the in-place write
// fails as well, both failures are returned.
func TestWriteFileAtomicOrInPlace_InPlaceFailureReportsBoth(t *testing.T) {
	target := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	atomicErr := &os.LinkError{Op: "rename", Err: syscall.EBUSY}
	inPlaceErr := &os.PathError{Op: "open", Path: target, Err: syscall.EROFS}

	mock := NewMockFileSystem(gomock.NewController(t))
	mock.EXPECT().WriteFileAtomic(gomock.Any(), gomock.Any(), gomock.Any()).Return(atomicErr)
	mock.EXPECT().Stat(target).DoAndReturn(os.Stat)
	mock.EXPECT().WriteFile(target, gomock.Any(), gomock.Any()).Return(inPlaceErr)

	err := WriteFileAtomicOrInPlace(mock, target, []byte("new"), 0o600)
	require.ErrorIs(t, err, syscall.EBUSY)
	require.ErrorIs(t, err, syscall.EROFS)
}

// TestResolveSymlinkTarget verifies a symlink is resolved to the file it points to, so a write
// through it keeps the link, and that a non-link path is returned unchanged.
func TestResolveSymlinkTarget(t *testing.T) {
	// Resolve the temporary directory itself (macOS links /var to /private/var).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	plain := filepath.Join(dir, "plain.json")
	require.NoError(t, os.WriteFile(plain, []byte("{}"), 0o600))
	assert.Equal(t, plain, ResolveSymlinkTarget(plain))

	missing := filepath.Join(dir, "missing.json")
	assert.Equal(t, missing, ResolveSymlinkTarget(missing))

	target := filepath.Join(dir, "real", "cache.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	// A dangling link (target not created yet) resolves link by link to its target.
	assert.Equal(t, target, ResolveSymlinkTarget(link))

	require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
	assert.Equal(t, target, ResolveSymlinkTarget(link))

	// A cycle is returned unchanged.
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.Symlink(b, a))
	require.NoError(t, os.Symlink(a, b))
	assert.Equal(t, a, ResolveSymlinkTarget(a))

	// A relative link resolves against the link's directory.
	relative := filepath.Join(dir, "relative.json")
	require.NoError(t, os.Symlink(filepath.Join("real", "cache.json"), relative))
	assert.Equal(t, target, ResolveSymlinkTarget(relative))
}

// assertSamePerm checks the permission bits of path, skipping the check where the OS does not
// keep Unix permission bits.
func assertSamePerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		return
	}
	assert.Equal(t, want, info.Mode().Perm(), "the in-place write keeps the existing permission bits")
}
