//go:build windows

package homedir

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirWindows_UNCUserprofile verifies that a UNC path set as USERPROFILE
// is returned unchanged by dirWindows(). toDriveAbsolute must not prepend
// HOMEDRIVE to an already-fully-qualified UNC path.
func TestDirWindows_UNCUserprofile(t *testing.T) {
	Reset()
	defer Reset()

	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", `\\server\share\user`)
	t.Setenv("HOMEDRIVE", "C:")
	t.Setenv("HOMEPATH", "")

	dir, err := dirWindows()
	require.NoError(t, err)
	assert.Equal(t, `\\server\share\user`, dir,
		"UNC USERPROFILE must be returned unchanged; toDriveAbsolute must not prepend HOMEDRIVE.")
}

// TestDirWindows_DriveRelativeHome verifies that HOME="C:Users\me" (drive-letter
// without a root backslash) is normalized to the drive-absolute "C:\Users\me".
func TestDirWindows_DriveRelativeHome(t *testing.T) {
	Reset()
	defer Reset()

	t.Setenv("HOME", `C:Users\me`)
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	dir, err := dirWindows()
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\me`, dir,
		"drive-letter-relative HOME must be normalized to drive-absolute path.")
}

// TestExpand_WindowsBackslashSubdir_BuildTagged verifies that
// Expand("~\subdir\file.txt") correctly expands to filepath.Join(Dir(),
// "subdir", "file.txt") on Windows. This is the build-tag counterpart of
// the runtime-skip version in homedir_test.go.
func TestExpand_WindowsBackslashSubdir_BuildTagged(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	SetDisableCache(true)
	defer SetDisableCache(false)

	result, err := Expand(`~\subdir\file.txt`)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "subdir", "file.txt"), result,
		`Expand("~\subdir\file.txt") must expand under Dir() on Windows.`)
}
