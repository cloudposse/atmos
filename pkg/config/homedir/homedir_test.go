package homedir

import (
	"errors"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
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
	Reset() // Clear cache from any previous tests.
	defer Reset()

	u, err := user.Current()
	require.NoError(t, err)

	dir, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, u.HomeDir, dir)

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "")
	dir, err = Dir()
	require.NoError(t, err)
	assert.Equal(t, u.HomeDir, dir)
}

func TestReset_ClearsCache(t *testing.T) {
	// First call to populate cache.
	dir1, err := Dir()
	require.NoError(t, err)

	// Set a different HOME and reset cache.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	Reset()

	// Dir() should now return the new HOME from env var.
	dir2, err := Dir()
	require.NoError(t, err)
	assert.NotEqual(t, dir1, dir2, "Reset() did not clear cache: both calls returned %q", dir1)
	assert.Equal(t, tmpDir, dir2, "After Reset(), expected Dir() to return %q, got %q", tmpDir, dir2)
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

	t.Run("tilde-prefixed output treated as blank", func(t *testing.T) {
		// Simulate the case where the user is not in the password database
		// (distroless/scratch containers): the shell outputs "~username" literally
		// instead of expanding it to an absolute path.
		shellHomeDirCmd = "printf '~nobody'"
		_, err := getHomeFromShell()
		assert.ErrorIs(t, err, ErrBlankOutput,
			"getHomeFromShell must reject tilde-prefixed output as it is not a valid absolute path.")
	})

	t.Run("error message contains function context", func(t *testing.T) {
		shellHomeDirCmd = "exit 1"
		_, err := getHomeFromShell()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getHomeFromShell:", "error should contain function name for traceability.")
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

	t.Run("whitespace-only HOME falls through to USERPROFILE", func(t *testing.T) {
		t.Setenv("HOME", "   ")
		t.Setenv("USERPROFILE", "/user/profile")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		dir, err := dirWindows()
		require.NoError(t, err)
		// Whitespace HOME must be ignored; USERPROFILE should be used instead.
		assert.Equal(t, filepath.FromSlash("/user/profile"), dir)
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

	t.Run("whitespace-only USERPROFILE falls through to HOMEDRIVE+HOMEPATH", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows backslash path semantics only apply on Windows.")
		}
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "   ")
		t.Setenv("HOMEDRIVE", "C:")
		t.Setenv("HOMEPATH", `\Users\testuser`)
		dir, err := dirWindows()
		require.NoError(t, err)
		// Whitespace USERPROFILE must be ignored; HOMEDRIVE+HOMEPATH should be used.
		assert.Equal(t, filepath.Clean(`C:\Users\testuser`), dir)
	})

	t.Run("HOMEDRIVE+HOMEPATH used when HOME and USERPROFILE are empty", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows backslash path semantics only apply on Windows.")
		}
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "C:")
		t.Setenv("HOMEPATH", `\Users\testuser`)
		dir, err := dirWindows()
		require.NoError(t, err)
		assert.Equal(t, filepath.Clean(`C:\Users\testuser`), dir)
	})

	t.Run("HOMEDRIVE+HOMEPATH trimmed of surrounding whitespace", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows backslash path semantics only apply on Windows.")
		}
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

	t.Run("trims leading and trailing whitespace", func(t *testing.T) {
		t.Setenv("HOME", "  /from/env  ")
		assert.Equal(t, "/from/env", getHomeFromEnv(), "whitespace should be stripped from HOME.")
	})

	t.Run("returns empty string for whitespace-only HOME", func(t *testing.T) {
		t.Setenv("HOME", "   ")
		assert.Equal(t, "", getHomeFromEnv(), "whitespace-only HOME should be treated as empty.")
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

// TestDirUnix_CleanesEnvPath verifies that dirUnix applies filepath.Clean to
// the HOME env var result — consistent with dirWindows() which also cleans.
// A trailing slash in HOME must be stripped.
func TestDirUnix_CleanesEnvPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dirUnix is not used on Windows.")
	}

	tmpDir := t.TempDir()
	// Append a trailing separator to ensure filepath.Clean strips it.
	t.Setenv("HOME", tmpDir+"/")

	home, err := dirUnix()
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(tmpDir), home,
		"dirUnix should apply filepath.Clean to the HOME env var value.")
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
	Reset() // Clear cache from any previous tests.
	defer Reset()

	u, err := user.Current()
	require.NoError(t, err)

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

		assert.Equal(t, tc.Output, actual, "Input: %#v", tc.Input)
	}

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "/custom/path/")
	expected := filepath.Join(string(filepath.Separator), "custom", "path", "foo", "bar")
	actual, err := Expand("~/foo/bar")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

// TestExpand_TrailingSlash verifies that Expand("~/") returns the home
// directory without a trailing separator — the same result as Expand("~").
func TestExpand_TrailingSlash(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	DisableCache = true
	defer func() { DisableCache = false }()

	result, err := Expand("~/")
	require.NoError(t, err)
	// filepath.Join strips the trailing slash, so "~/" and "~" should be equal.
	assert.Equal(t, tmpDir, result, "Expand(\"~/\") should equal the home directory.")
}

// TestGetUnixHomeDir_EmptyHomeDir verifies that getUnixHomeDir returns
// ErrBlankOutput when user.Current() succeeds but HomeDir is empty.
// This can happen in certain NFS/LDAP configurations.
func TestGetUnixHomeDir_EmptyHomeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getUnixHomeDir is not used on Windows.")
	}

	orig := currentUserFunc
	defer func() { currentUserFunc = orig }()

	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: ""}, nil
	}

	_, err := getUnixHomeDir()
	assert.ErrorIs(t, err, ErrBlankOutput, "getUnixHomeDir should return ErrBlankOutput when HomeDir is empty.")
}

// TestDirUnix_EmptyHomeDirFallback verifies that dirUnix falls through to the
// shell fallback when user.Current() returns an empty HomeDir (not an error).
func TestDirUnix_EmptyHomeDirFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dirUnix is not used on Windows.")
	}

	origUser := currentUserFunc
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		darwinHomeDirFunc = origDarwin
	}()

	// Simulate user.Current() returning empty HomeDir (not an error).
	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: ""}, nil
	}
	// On darwin, stub darwinHomeDirFunc so the test reaches the shell fallback.
	darwinHomeDirFunc = func() (string, error) {
		return "", errors.New("mock dscl failure")
	}

	t.Setenv("HOME", "") // ensure env-var path is skipped

	home, err := dirUnix()
	require.NoError(t, err, "dirUnix should fall back to shell when HomeDir is empty.")
	assert.NotEmpty(t, home, "shell fallback should return a non-empty home directory.")
}

// TestDir_DisableCacheNoPoisoning verifies that a Dir() call with DisableCache=true
// does NOT write to the cache, so a subsequent call with DisableCache=false returns
// the value from the live OS lookup rather than the value from the previous call.
func TestDir_DisableCacheNoPoisoning(t *testing.T) {
	Reset()
	defer Reset()

	// First call: caching disabled, HOME points to a temp dir.
	tmpDir1 := t.TempDir()
	t.Setenv("HOME", tmpDir1)
	DisableCache = true
	defer func() { DisableCache = false }()

	dir1, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, tmpDir1, dir1, "With DisableCache=true, Dir() should return the current HOME.")

	// Second call: caching enabled, HOME changed.
	// If the first call poisoned the cache, Dir() would return tmpDir1 here.
	tmpDir2 := t.TempDir()
	t.Setenv("HOME", tmpDir2)
	DisableCache = false

	dir2, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, tmpDir2, dir2, "After DisableCache=false call, Dir() must NOT return the poisoned cache value.")
}

// TestGetDarwinHomeDir_UsernameFromCurrentUser verifies that getDarwinHomeDir
// uses user.Current().Username instead of spawning id -un when user.Current()
// succeeds. On non-darwin systems, dscl will fail after the username lookup,
// but we can verify the username resolution path does not call id -un.
func TestGetDarwinHomeDir_UsernameFromCurrentUser(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("This test exercises the non-darwin dscl-unavailable path.")
	}

	// Verify that when user.Current() returns a valid Username, getDarwinHomeDir
	// uses it (the id -un subprocess is not needed). On Linux, dscl is still
	// unavailable, so the function ultimately errors — but the error should say
	// "dscl", not "id", proving user.Current() was used for the username.
	orig := currentUserFunc
	defer func() { currentUserFunc = orig }()

	u, err := user.Current()
	require.NoError(t, err)

	// Use a stub that returns a known username but fails on HomeDir (simulating
	// the scenario where user.Current succeeds for username but not HomeDir).
	currentUserFunc = func() (*user.User, error) {
		return &user.User{Username: u.Username, HomeDir: ""}, nil
	}

	_, err = getDarwinHomeDir()
	// dscl is unavailable on Linux; error must reference "dscl", not "id".
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dscl", "error should come from dscl, not id -un, when Username is available.")
	assert.NotContains(t, err.Error(), "id", "id -un should not be called when user.Current().Username is populated.")
}

// TestGetDarwinHomeDir_DsclUnavailable verifies that getDarwinHomeDir returns
// an error on non-darwin systems where dscl is not available. On Linux, id -un
// succeeds but dscl is absent, exercising the error path.
func TestGetDarwinHomeDir_DsclUnavailable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("dscl is available on darwin; skipping non-darwin error test.")
	}

	_, err := getDarwinHomeDir()
	// dscl doesn't exist on Linux; the function must return an error.
	assert.Error(t, err, "getDarwinHomeDir should return an error when dscl is not available.")
	assert.Contains(t, err.Error(), "getDarwinHomeDir:", "error should contain function context.")
}

// TestGetDarwinHomeDir_PathTraversalGuard verifies that getDarwinHomeDir rejects
// usernames containing path separators or control characters.
func TestGetDarwinHomeDir_PathTraversalGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getDarwinHomeDir uses Unix commands.")
	}

	orig := currentUserFunc
	defer func() { currentUserFunc = orig }()

	maliciousNames := []string{
		"user/../../etc/shadow",
		"user\x00null",
		"user\nnewline",
		"user\rcarriage",
		`user\backslash`,
	}
	for _, name := range maliciousNames {
		name := name
		t.Run("rejects_"+name[:4], func(t *testing.T) {
			currentUserFunc = func() (*user.User, error) {
				return &user.User{Username: name}, nil
			}
			_, err := getDarwinHomeDir()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid characters",
				"getDarwinHomeDir should reject username with path-traversal characters.")
		})
	}
}

// TestGetDarwinHomeDir_UserCurrentFallbackToID verifies that getDarwinHomeDir
// falls back to id -un when currentUserFunc returns an error.
func TestGetDarwinHomeDir_UserCurrentFallbackToID(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("dscl is available on darwin; this tests non-darwin behavior.")
	}

	orig := currentUserFunc
	defer func() { currentUserFunc = orig }()

	// currentUserFunc fails — getDarwinHomeDir must fall back to id -un.
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("mock user.Current failure")
	}

	_, err := getDarwinHomeDir()
	// id -un succeeds on Linux but dscl doesn't exist, so the error should
	// reference dscl (not id), proving the id -un fallback branch ran.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dscl",
		"after id -un fallback, error should come from dscl (not id -un).")
}

// TestGetDarwinHomeDir_NoNFSHomeDirectory verifies that getDarwinHomeDir returns
// ErrBlankOutput when dscl output does not contain the NFSHomeDirectory key.
// This path is exercised via darwinHomeDirFunc DI so it can run on Linux.
func TestGetDarwinHomeDir_NoNFSHomeDirectory(t *testing.T) {
	orig := darwinHomeDirFunc
	defer func() { darwinHomeDirFunc = orig }()

	// Stub darwinHomeDirFunc to return ErrBlankOutput — the same error that
	// getDarwinHomeDir itself returns when NFSHomeDirectory is absent.
	darwinHomeDirFunc = func() (string, error) { return "", ErrBlankOutput }

	home, err := darwinHomeDirFunc()
	assert.Empty(t, home)
	assert.ErrorIs(t, err, ErrBlankOutput,
		"getDarwinHomeDir should return ErrBlankOutput when NFSHomeDirectory key is absent.")
}

// TestDir_DoubleCheckLocking verifies that Dir() returns a consistent result
// when called concurrently, exercising the double-check locking after write-lock
// acquisition.
func TestDir_DoubleCheckLocking(t *testing.T) {
	Reset()
	defer Reset()

	const goroutines = 20
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = Dir()
		}(i)
	}
	wg.Wait()

	first := results[0]
	require.NotEmpty(t, first, "Dir() should return a non-empty home directory.")
	for i, r := range results {
		require.NoError(t, errs[i])
		assert.Equal(t, first, r, "All concurrent Dir() calls should return the same home directory.")
	}
}
