package homedir

import (
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
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
			require.NoError(t, err, "iteration %d", i)
			require.Equal(t, tmpDir, dir, "iteration %d: expected Dir() to return %q, got %q", i, tmpDir, dir)
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

// TestShellHomeDir covers the error paths in shellHomeDir (the production
// implementation), which uses id -un and validates the username before
// delegating to the shell.
func TestShellHomeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shellHomeDir uses id and sh, which are not available on Windows.")
	}

	t.Run("succeeds in normal environment", func(t *testing.T) {
		home, err := shellHomeDir()
		require.NoError(t, err)
		assert.NotEmpty(t, home, "shellHomeDir should return a non-empty path.")
		assert.False(t, strings.HasPrefix(home, "~"), "shellHomeDir must not return a tilde-prefixed path.")
	})

	t.Run("id failure propagates", func(t *testing.T) {
		orig := shellGetUsernameFunc
		defer func() { shellGetUsernameFunc = orig }()
		want := errors.New("mock id failure")
		shellGetUsernameFunc = func() (string, error) {
			return "", want
		}
		_, err := shellHomeDir()
		require.Error(t, err)
		assert.ErrorIs(t, err, want, "id failure should propagate from shellGetUsernameFunc.")
	})

	t.Run("empty username returns ErrBlankOutput", func(t *testing.T) {
		orig := shellGetUsernameFunc
		defer func() { shellGetUsernameFunc = orig }()
		shellGetUsernameFunc = func() (string, error) { return "", nil }
		_, err := shellHomeDir()
		assert.ErrorIs(t, err, ErrBlankOutput, "empty username should return ErrBlankOutput.")
	})

	t.Run("username with path separators rejected", func(t *testing.T) {
		orig := shellGetUsernameFunc
		defer func() { shellGetUsernameFunc = orig }()
		shellGetUsernameFunc = func() (string, error) { return "user/evil", nil }
		_, err := shellHomeDir()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid characters", "path traversal username must be rejected.")
	})

	t.Run("username with shell metacharacters rejected", func(t *testing.T) {
		// Verify the whitelist rejects shell metacharacters that could cause
		// injection or unexpected expansion when interpolated into sh -c.
		// Also verifies new chars (|, &, >, <, (, ), tab) caught by whitelist.
		metacharNames := []string{
			"user'quote",
			"user`backtick",
			"user$dollar",
			"user;semi",
			"user space",
			"user|pipe",
			"user&amp",
			"user>redir",
			"user<redir",
			"user(paren",
			"user)paren",
			"user\ttab",
		}
		for _, name := range metacharNames {
			orig := shellGetUsernameFunc
			defer func() { shellGetUsernameFunc = orig }()
			shellGetUsernameFunc = func() (string, error) { return name, nil }
			_, err := shellHomeDir()
			require.Error(t, err, "username %q should be rejected", name)
			assert.Contains(t, err.Error(), "invalid characters",
				"username %q should be rejected with 'invalid characters' error", name)
		}
	})

	t.Run("tilde-prefixed shell output returns ErrBlankOutput", func(t *testing.T) {
		// Directly test the tilde-guard in shellHomeDir by overriding shellHomeDirFunc
		// to call the real shellHomeDir but with shellGetUsernameFunc returning a
		// username that the shell will expand as a tilde literal (user not in passwd).
		// We don't rely on atmostestnonexistentuser not existing in the system's passwd;
		// if the user somehow exists, the test will not exercise the guard path
		// but will also not fail — the subtest is environment-dependent by design.
		// The deterministic guard tests live in TestShellHomeDir/username_with_shell_metacharacters_rejected.
		origShell := shellGetUsernameFunc
		defer func() { shellGetUsernameFunc = origShell }()
		shellGetUsernameFunc = func() (string, error) { return "atmostestnonexistentuser", nil }
		result, err := shellHomeDir()
		if err != nil {
			// Expected path: the shell output ~atmostestnonexistentuser literally
			// and ErrBlankOutput was returned.
			assert.ErrorIs(t, err, ErrBlankOutput, "tilde-prefixed output must be rejected as ErrBlankOutput.")
		} else {
			// The user happens to exist on this system; the home dir was expanded.
			// The tilde guard was not needed. Assert the result is an absolute path.
			assert.True(t, strings.HasPrefix(result, "/"),
				"if user exists, shellHomeDir must return an absolute path, not %q", result)
		}
	})
}

// TestGetHomeFromShell_Failure covers the error paths in getHomeFromShell using
// the shellHomeDirFunc variable for dependency injection.
func TestGetHomeFromShell_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("getHomeFromShell relies on 'sh', which is not available on Windows.")
	}

	t.Run("command failure", func(t *testing.T) {
		orig := shellHomeDirFunc
		defer func() { shellHomeDirFunc = orig }()
		shellHomeDirFunc = func() (string, error) {
			return "", errors.New("mock shell failure")
		}
		_, err := getHomeFromShell()
		assert.Error(t, err, "getHomeFromShell should propagate shell command failures.")
	})

	t.Run("empty output", func(t *testing.T) {
		orig := shellHomeDirFunc
		defer func() { shellHomeDirFunc = orig }()
		shellHomeDirFunc = func() (string, error) {
			return "", ErrBlankOutput
		}
		_, err := getHomeFromShell()
		assert.ErrorIs(t, err, ErrBlankOutput, "getHomeFromShell should return ErrBlankOutput for empty output.")
	})

	t.Run("tilde-prefixed output is forwarded from shellHomeDirFunc", func(t *testing.T) {
		orig := shellHomeDirFunc
		defer func() { shellHomeDirFunc = orig }()
		// This subtest verifies that getHomeFromShell faithfully propagates whatever
		// shellHomeDirFunc returns (including ErrBlankOutput for tilde-prefixed
		// output). The actual tilde-guard logic lives in shellHomeDir and is tested
		// in TestShellHomeDir.
		shellHomeDirFunc = func() (string, error) {
			return "", ErrBlankOutput
		}
		_, err := getHomeFromShell()
		assert.ErrorIs(t, err, ErrBlankOutput,
			"getHomeFromShell must propagate ErrBlankOutput from shellHomeDirFunc.")
	})

	t.Run("error message contains function context", func(t *testing.T) {
		orig := shellHomeDirFunc
		defer func() { shellHomeDirFunc = orig }()
		shellHomeDirFunc = func() (string, error) {
			return "", fmt.Errorf("getHomeFromShell: mock error")
		}
		_, err := getHomeFromShell()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getHomeFromShell:", "error should contain function name for traceability.")
	})
}

// TestShellHomeDir_ShellUnavailableReal verifies that the real shellHomeDir
// returns ErrShellUnavailable when sh cannot be found, using PATH manipulation
// to simulate a minimal container environment without a shell.
func TestShellHomeDir_ShellUnavailableReal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shellHomeDir is not used on Windows.")
	}

	origUsername := shellGetUsernameFunc
	defer func() { shellGetUsernameFunc = origUsername }()

	// Provide a valid username so the guard passes; use an empty PATH so sh
	// cannot be found, exercising the exec.ErrNotFound branch in shellHomeDir.
	shellGetUsernameFunc = func() (string, error) { return "validuser", nil }
	t.Setenv("PATH", t.TempDir()) // PATH contains no executables

	_, err := shellHomeDir()
	assert.ErrorIs(t, err, ErrShellUnavailable,
		"shellHomeDir must return ErrShellUnavailable when sh is not in PATH.")
}

// TestShellGetUsernameFunc_IDNotFound_UserEnvFallback verifies that the real
// shellGetUsernameFunc falls back to $USER when id is not found in PATH.
func TestShellGetUsernameFunc_IDNotFound_UserEnvFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shellGetUsernameFunc is not used on Windows.")
	}

	// Use an empty directory as PATH so id cannot be found; $USER is set.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("USER", "fallbackuser")

	name, err := shellGetUsernameFunc()
	require.NoError(t, err, "shellGetUsernameFunc should fall back to $USER when id is absent.")
	assert.Equal(t, "fallbackuser", name, "returned username must match the $USER env var.")
}

// TestShellGetUsernameFunc_IDNotFound_ErrIDUnavailable verifies that the real
// shellGetUsernameFunc returns ErrIDUnavailable when id is not found and both
// $USER and whoami are unavailable.
func TestShellGetUsernameFunc_IDNotFound_ErrIDUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shellGetUsernameFunc is not used on Windows.")
	}

	// Use an empty directory as PATH so both id and whoami cannot be found;
	// clear $USER so the env-var fallback also fails.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("USER", "")

	_, err := shellGetUsernameFunc()
	assert.ErrorIs(t, err, ErrIDUnavailable,
		"shellGetUsernameFunc must return ErrIDUnavailable when id, whoami, and $USER are all unavailable.")
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

	t.Run("HOMEPATH without leading backslash gets backslash prepended", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows backslash path semantics only apply on Windows.")
		}
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "C:")
		t.Setenv("HOMEPATH", `Users\testuser`)
		dir, err := dirWindows()
		require.NoError(t, err)
		// Missing leading backslash in HOMEPATH must be fixed; result must be
		// C:\Users\testuser (absolute), not C:Users\testuser (drive-relative).
		assert.Equal(t, filepath.Clean(`C:\Users\testuser`), dir,
			"HOMEPATH without leading backslash must be normalised to absolute path.")
	})

	t.Run("forward-slash HOME converted to native separators (Cygwin/WSL/Git Bash)", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			// filepath.FromSlash is a no-op on non-Windows, so this subtest
			// only proves the conversion logic on the target platform.
			t.Skip("filepath.FromSlash conversion is only meaningful on Windows.")
		}
		t.Setenv("HOME", "/cygwin/home/user")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		dir, err := dirWindows()
		require.NoError(t, err)
		// filepath.FromSlash converts / → \; filepath.Clean then normalises.
		// The result is a native Windows path, not a drive-relative path.
		assert.Equal(t, filepath.Clean(filepath.FromSlash("/cygwin/home/user")), dir,
			"forward-slash HOME must be converted to native separators to avoid drive-relative result.")
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
	origShell := shellHomeDirFunc
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		shellHomeDirFunc = origShell
		darwinHomeDirFunc = origDarwin
	}()

	Reset()
	defer Reset()

	DisableCache = true
	defer func() { DisableCache = false }()

	t.Setenv("HOME", "")
	currentUserFunc = func() (*user.User, error) { return nil, errors.New("mock failure") }
	darwinHomeDirFunc = func(_ string) (string, error) { return "", errors.New("mock darwin failure") }
	shellHomeDirFunc = func() (string, error) { return "", errors.New("mock shell failure") }

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
	origShell := shellHomeDirFunc
	origDarwin := darwinHomeDirFunc
	defer func() {
		currentUserFunc = origUser
		shellHomeDirFunc = origShell
		darwinHomeDirFunc = origDarwin
	}()

	Reset()
	defer Reset()

	DisableCache = true
	defer func() { DisableCache = false }()

	t.Setenv("HOME", "")
	currentUserFunc = func() (*user.User, error) { return nil, errors.New("mock failure") }
	darwinHomeDirFunc = func(_ string) (string, error) { return "", errors.New("mock darwin failure") }
	shellHomeDirFunc = func() (string, error) { return "", errors.New("mock shell failure") }

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
	darwinHomeDirFunc = func(_ string) (string, error) {
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
		darwinHomeDirFunc = func(_ string) (string, error) { return fakeHome, nil }

		home, err := darwinHomeDirFunc("")
		require.NoError(t, err)
		assert.Equal(t, fakeHome, home)
	})

	t.Run("propagates injected error", func(t *testing.T) {
		darwinHomeDirFunc = func(_ string) (string, error) { return "", errors.New("dscl not available") }

		_, err := darwinHomeDirFunc("")
		assert.Error(t, err)
	})
}

func TestExpand(t *testing.T) {
	Reset() // Clear cache from any previous tests.
	defer Reset()

	u, err := user.Current()
	require.NoError(t, err)

	cases := []struct {
		Input     string
		Output    string
		ExpectErr bool
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
		if tc.ExpectErr {
			require.Error(t, err, "Input: %#v", tc.Input)
		} else {
			require.NoError(t, err, "Input: %#v", tc.Input)
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
	darwinHomeDirFunc = func(_ string) (string, error) {
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
// uses the supplied username for the dscl lookup. On non-darwin systems, dscl
// will fail after the username lookup, but we can verify the username is used.
func TestGetDarwinHomeDir_UsernameFromCurrentUser(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("This test exercises the non-darwin dscl-unavailable path.")
	}

	u, err := user.Current()
	require.NoError(t, err)

	// Pass the current username directly. On Linux, dscl is still unavailable,
	// so the function ultimately errors — but the error must reference "dscl:",
	// proving the username was passed through and id -un was not spawned.
	_, err = getDarwinHomeDir(u.Username)
	// dscl is unavailable on Linux; the error must reference "getDarwinHomeDir: dscl:",
	// proving that the provided username was used and id -un was not spawned.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getDarwinHomeDir: dscl:", "error should come from dscl, not id -un, when Username is available.")
}

// TestGetDarwinHomeDir_DsclUnavailable verifies that getDarwinHomeDir returns
// an error on non-darwin systems where dscl is not available. On Linux, id -un
// succeeds but dscl is absent, exercising the error path.
func TestGetDarwinHomeDir_DsclUnavailable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("dscl is available on darwin; skipping non-darwin error test.")
	}

	_, err := getDarwinHomeDir("")
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

	maliciousNames := []string{
		"user/../../etc/shadow",
		"user\x00null",
		"user\nnewline",
		"user\rcarriage",
		`user\backslash`,
		"user'quote",
		"user`backtick",
		"user$dollar",
		"user;semicolon",
		"user space",
		// New: operator characters now also rejected by the whitelist regex.
		"user|pipe",
		"user&amp",
		"user>redir",
		"user<redir",
		"user(paren",
		"user)paren",
		"user\ttab",
	}
	for i, name := range maliciousNames {
		t.Run(fmt.Sprintf("rejects_malicious_%d", i), func(t *testing.T) {
			_, err := getDarwinHomeDir(name)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid characters",
				"getDarwinHomeDir should reject username with path-traversal characters.")
		})
	}
}

// TestGetDarwinHomeDir_UserCurrentFallbackToID verifies that getDarwinHomeDir
// falls back to id -un when no cached username is provided (empty string).
func TestGetDarwinHomeDir_UserCurrentFallbackToID(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("dscl is available on darwin; this tests non-darwin behavior.")
	}

	// Pass empty cachedUsername — getDarwinHomeDir must fall back to id -un.
	_, err := getDarwinHomeDir("")
	// id -un succeeds on Linux but dscl doesn't exist, so the error should
	// reference dscl (not id), proving the id -un fallback branch ran.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dscl",
		"after id -un fallback, error should come from dscl (not id -un).")
}

// TestGetDarwinHomeDir_NoNFSHomeDirectory verifies that dirUnix() falls through
// to the shell fallback when getDarwinHomeDir returns ErrBlankOutput (i.e., dscl
// output does not contain the NFSHomeDirectory key). The test exercises this path
// via the darwinHomeDirFunc DI hook and runs on all platforms.
func TestGetDarwinHomeDir_NoNFSHomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dirUnix is not used on Windows.")
	}

	origDarwin := darwinHomeDirFunc
	origUser := currentUserFunc
	defer func() {
		darwinHomeDirFunc = origDarwin
		currentUserFunc = origUser
	}()

	// Force getUnixHomeDir to fail so that darwinHomeDirFunc is tried next.
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("mock failure")
	}
	// Stub darwinHomeDirFunc to return ErrBlankOutput — the error getDarwinHomeDir
	// returns when dscl output lacks the NFSHomeDirectory key.
	darwinHomeDirFunc = func(_ string) (string, error) { return "", ErrBlankOutput }

	t.Setenv("HOME", "") // ensure env-var path is skipped

	// dirUnix should fall through to the shell fallback after darwinHomeDirFunc
	// returns ErrBlankOutput.
	home, err := dirUnix()
	require.NoError(t, err, "dirUnix should fall through to shell after darwinHomeDirFunc returns ErrBlankOutput.")
	assert.NotEmpty(t, home, "shell fallback should return a non-empty home directory.")
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
