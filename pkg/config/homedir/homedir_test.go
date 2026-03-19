package homedir

import (
	"errors"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func BenchmarkDir(b *testing.B) {
	// We do this for any "warmups"
	for i := 0; i < 10; i++ {
		Dir()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Dir()
	}
}

func TestDir(t *testing.T) {
	Reset() // Clear cache from any previous tests
	defer Reset()

	u, err := user.Current()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if u.HomeDir != dir {
		t.Fatalf("%#v != %#v", u.HomeDir, dir)
	}

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "")
	dir, err = Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if u.HomeDir != dir {
		t.Fatalf("%#v != %#v", u.HomeDir, dir)
	}
}

func TestReset_ClearsCache(t *testing.T) {
	// First call to populate cache.
	dir1, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Set a different HOME and reset cache.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	Reset()

	// Dir() should now return the new HOME from env var.
	dir2, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if dir1 == dir2 {
		t.Fatalf("Reset() did not clear cache: both calls returned %q", dir1)
	}

	if dir2 != tmpDir {
		t.Fatalf("After Reset(), expected Dir() to return %q, got %q", tmpDir, dir2)
	}
}

func TestReset_WorksAcrossMultipleTests(t *testing.T) {
	// This test reproduces the issue where Reset() doesn't work properly
	// when tests are run multiple times (go test -count=2).

	for i := 0; i < 3; i++ {
		t.Run("iteration", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			Reset()

			dir, err := Dir()
			if err != nil {
				t.Fatalf("iteration %d: err: %s", i, err)
			}

			if dir != tmpDir {
				t.Fatalf("iteration %d: expected Dir() to return %q, got %q", i, tmpDir, dir)
			}
		})
	}
}

// TestGetUnixHomeDir verifies that getUnixHomeDir returns the current user's home
// directory via os/user.Current (the code changed by the CodeQL #5157 fix).
func TestGetUnixHomeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getUnixHomeDir is not used on Windows.")
	}

	u, err := user.Current()
	require.NoError(t, err)

	home, err := getUnixHomeDir()
	require.NoError(t, err)
	assert.Equal(t, u.HomeDir, home, "getUnixHomeDir should return the current user's home directory.")
}

// TestGetHomeFromShell verifies the shell-based home directory fallback.
func TestGetHomeFromShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getHomeFromShell relies on 'sh -c', which is not available on Windows.")
	}

	home, err := getHomeFromShell()
	require.NoError(t, err)
	assert.NotEmpty(t, home, "getHomeFromShell should return a non-empty path.")
}

// TestGetHomeFromShell_Failure covers the error paths in getHomeFromShell using
// the shellHomeDirCmd variable for dependency injection.
func TestGetHomeFromShell_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getHomeFromShell relies on 'sh', which is not available on Windows.")
	}

	orig := shellHomeDirCmd
	defer func() { shellHomeDirCmd = orig }()

	t.Run("command failure", func(t *testing.T) {
		shellHomeDirCmd = "exit 1" // forces cmd.Run() to return an error
		_, err := getHomeFromShell()
		assert.Error(t, err, "getHomeFromShell should propagate shell command failures.")
	})

	t.Run("empty output", func(t *testing.T) {
		shellHomeDirCmd = "printf ''" // forces empty trimmed output (POSIX compliant, unlike "echo -n")
		_, err := getHomeFromShell()
		assert.ErrorIs(t, err, ErrBlankOutput, "getHomeFromShell should return ErrBlankOutput for empty output.")
	})
}

func TestDirWindows(t *testing.T) {
	t.Run("HOME env var wins", func(t *testing.T) {
		t.Setenv("HOME", "/my/home")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		dir, err := dirWindows()
		require.NoError(t, err)
		assert.Equal(t, filepath.FromSlash("/my/home"), dir)
	})

	t.Run("USERPROFILE wins when HOME is empty", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "/user/profile")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		dir, err := dirWindows()
		require.NoError(t, err)
		assert.Equal(t, filepath.FromSlash("/user/profile"), dir)
	})

	t.Run("HOMEDRIVE+HOMEPATH used when HOME and USERPROFILE are empty", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "C:")
		t.Setenv("HOMEPATH", `\Users\testuser`)
		dir, err := dirWindows()
		require.NoError(t, err)
		assert.Equal(t, filepath.Clean(`C:\Users\testuser`), dir)
	})

	t.Run("HOMEDRIVE+HOMEPATH trimmed of surrounding whitespace", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "  C:  ")
		t.Setenv("HOMEPATH", `  \Users\testuser  `)
		dir, err := dirWindows()
		require.NoError(t, err)
		assert.Equal(t, filepath.Clean(`C:\Users\testuser`), dir)
	})

	t.Run("error when all env vars are empty", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		_, err := dirWindows()
		assert.ErrorIs(t, err, ErrHomeDrivePathBlank)
	})
}

// TestGetHomeFromEnv tests the HOME env-var lookup (plan9 branch is not reachable
// in unit tests, but the common path is fully exercised here).
func TestGetHomeFromEnv(t *testing.T) {
	t.Run("returns HOME when set", func(t *testing.T) {
		t.Setenv("HOME", "/from/env")
		assert.Equal(t, "/from/env", getHomeFromEnv())
	})

	t.Run("returns empty string when HOME is unset", func(t *testing.T) {
		t.Setenv("HOME", "")
		assert.Equal(t, "", getHomeFromEnv())
	})
}

// TestDirUnix_FallbackToUnixHomeDir tests that dirUnix falls through to
// getUnixHomeDir when the HOME env var is not set.
func TestDirUnix_FallbackToUnixHomeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dirUnix is not used on Windows.")
	}

	t.Setenv("HOME", "")

	u, err := user.Current()
	require.NoError(t, err)

	home, err := dirUnix()
	require.NoError(t, err)
	assert.Equal(t, u.HomeDir, home)
}

// TestDir_Cache verifies that Dir() caches its result and returns it on
// subsequent calls without re-reading the home directory.
func TestDir_Cache(t *testing.T) {
	Reset()
	defer Reset()

	dir1, err := Dir()
	require.NoError(t, err)
	assert.NotEmpty(t, dir1)

	// Change HOME — Dir() should still return the cached value.
	t.Setenv("HOME", t.TempDir())

	dir2, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, dir1, dir2, "Dir() should return cached result when DisableCache is false.")
}

// TestExpand_WithDisabledCache verifies Expand falls back to Dir() properly
// with DisableCache enabled and HOME set to an explicit value.
func TestExpand_WithDisabledCache(t *testing.T) {
	Reset()
	defer Reset()

	DisableCache = true
	defer func() { DisableCache = false }()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	expected := filepath.Join(tmpDir, "subdir", "file.txt")
	actual, err := Expand("~/subdir/file.txt")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

// TestExpand_DirError verifies that Expand propagates errors from Dir().
func TestExpand_DirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, clear all env vars to make Dir() fail.
		Reset()
		defer Reset()

		DisableCache = true
		defer func() { DisableCache = false }()

		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")

		_, err := Expand("~/test")
		assert.Error(t, err)
		return
	}

	// On Unix, use dependency injection to force Dir() to return an error.
	// On darwin, darwinHomeDirFunc must also be mocked because dirUnix() tries
	// it before currentUserFunc, and the real dscl command succeeds on macOS.
	origUser := currentUserFunc
	origShell := shellHomeDirCmd
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		shellHomeDirCmd = origShell
		darwinHomeDirFunc = origDarwin
	}()

	Reset()
	defer Reset()

	DisableCache = true
	defer func() { DisableCache = false }()

	t.Setenv("HOME", "")
	currentUserFunc = func() (*user.User, error) { return nil, errors.New("mock failure") }
	darwinHomeDirFunc = func() (string, error) { return "", errors.New("mock darwin failure") }
	shellHomeDirCmd = "exit 1"

	_, err := Expand("~/test")
	assert.Error(t, err, "Expand should propagate Dir() errors.")
}

// TestDir_Error verifies that Dir() propagates errors from the underlying
// OS home-directory lookup when all methods fail.
func TestDir_Error(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("This test uses Unix-specific injection variables.")
	}

	// On darwin, darwinHomeDirFunc must also be mocked because dirUnix() tries
	// it before currentUserFunc, and the real dscl command succeeds on macOS.
	origUser := currentUserFunc
	origShell := shellHomeDirCmd
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		shellHomeDirCmd = origShell
		darwinHomeDirFunc = origDarwin
	}()

	Reset()
	defer Reset()

	DisableCache = true
	defer func() { DisableCache = false }()

	t.Setenv("HOME", "")
	currentUserFunc = func() (*user.User, error) { return nil, errors.New("mock failure") }
	darwinHomeDirFunc = func() (string, error) { return "", errors.New("mock darwin failure") }
	shellHomeDirCmd = "exit 1"

	_, err := Dir()
	assert.Error(t, err, "Dir() should return an error when all lookup methods fail.")
}

// TestDir_DisableCache verifies that setting DisableCache=true causes Dir()
// to re-read the home directory on each invocation instead of returning a
// stale cached value.
func TestDir_DisableCache(t *testing.T) {
	Reset()
	defer Reset()

	// Populate cache with the real home dir.
	_, err := Dir()
	require.NoError(t, err)

	// Switch to a temp dir and disable caching.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	DisableCache = true
	defer func() { DisableCache = false }()

	dir, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, tmpDir, dir, "With DisableCache=true, Dir() should re-read HOME each time.")
}

// TestExpandError_UserSpecificDir verifies the error path in Expand for ~username.
func TestExpandError_UserSpecificDir(t *testing.T) {
	_, err := Expand("~user/path")
	assert.ErrorIs(t, err, ErrCannotExpandHomeDir)
}

// TestExpand_TildeOnly verifies that Expand("~") returns just the home directory.
func TestExpand_TildeOnly(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	DisableCache = true
	defer func() { DisableCache = false }()

	result, err := Expand("~")
	require.NoError(t, err)
	assert.Equal(t, tmpDir, result)
}

// TestGetUnixHomeDir_Error covers the error path in getUnixHomeDir (the function
// modified by the CodeQL #5157 fix) using dependency injection via currentUserFunc.
func TestGetUnixHomeDir_Error(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getUnixHomeDir is not used on Windows.")
	}

	orig := currentUserFunc
	defer func() { currentUserFunc = orig }()

	want := errors.New("mock user.Current failure")
	currentUserFunc = func() (*user.User, error) { return nil, want }

	_, err := getUnixHomeDir()
	// getUnixHomeDir wraps the error from currentUserFunc with context.
	assert.ErrorIs(t, err, want, "getUnixHomeDir should propagate the error from currentUserFunc.")
}

// TestDirUnix_ShellFallback covers the getHomeFromShell() fallback path in
// dirUnix when getUnixHomeDir returns an error (HOME is also unset).
// On darwin, darwinHomeDirFunc is also stubbed to ensure the shell path is
// reached rather than the dscl path.
func TestDirUnix_ShellFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dirUnix is not used on Windows.")
	}

	origUser := currentUserFunc
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		darwinHomeDirFunc = origDarwin
	}()

	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("mock user.Current failure")
	}
	// On darwin, stub darwinHomeDirFunc so the test reaches the shell fallback.
	darwinHomeDirFunc = func() (string, error) {
		return "", errors.New("mock dscl failure")
	}

	t.Setenv("HOME", "") // ensure env-var path is skipped

	home, err := dirUnix()
	require.NoError(t, err, "dirUnix should fall back to getHomeFromShell when getUnixHomeDir fails.")
	assert.NotEmpty(t, home, "shell fallback should return a non-empty home directory.")
}

// TestDarwinHomeDirFunc verifies that darwinHomeDirFunc can be replaced with a
// stub, exercising the injection point used on macOS. Because runtime.GOOS
// cannot be changed at test time, the darwin branch inside dirUnix() itself
// cannot be reached on Linux/Windows; these tests cover the stub in isolation.
func TestDarwinHomeDirFunc(t *testing.T) {
	orig := darwinHomeDirFunc
	defer func() { darwinHomeDirFunc = orig }()

	t.Run("returns injected home dir", func(t *testing.T) {
		fakeHome := t.TempDir()
		darwinHomeDirFunc = func() (string, error) { return fakeHome, nil }

		home, err := darwinHomeDirFunc()
		require.NoError(t, err)
		assert.Equal(t, fakeHome, home)
	})

	t.Run("propagates injected error", func(t *testing.T) {
		darwinHomeDirFunc = func() (string, error) { return "", errors.New("dscl not available") }

		_, err := darwinHomeDirFunc()
		assert.Error(t, err)
	})
}

func TestExpand(t *testing.T) {
	Reset() // Clear cache from any previous tests
	defer Reset()

	u, err := user.Current()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	cases := []struct {
		Input  string
		Output string
		Err    bool
	}{
		{
			"/foo",
			"/foo",
			false,
		},

		{
			"~/foo",
			filepath.Join(u.HomeDir, "foo"),
			false,
		},

		{
			"",
			"",
			false,
		},

		{
			"~",
			u.HomeDir,
			false,
		},

		{
			"~foo/foo",
			"",
			true,
		},
	}

	for _, tc := range cases {
		actual, err := Expand(tc.Input)
		if (err != nil) != tc.Err {
			t.Fatalf("Input: %#v\n\nErr: %s", tc.Input, err)
		}

		if actual != tc.Output {
			t.Fatalf("Input: %#v\n\nOutput: %#v", tc.Input, actual)
		}
	}

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "/custom/path/")
	expected := filepath.Join(string(filepath.Separator), "custom", "path", "foo", "bar")
	actual, err := Expand("~/foo/bar")

	if err != nil {
		t.Errorf("No error is expected, got: %v", err)
	} else if actual != expected {
		t.Errorf("Expected: %v; actual: %v", expected, actual)
	}
}
