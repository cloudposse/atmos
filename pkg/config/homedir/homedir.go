// Package homedir implements home directory detection across Linux, macOS, and Windows.
//
// It uses a layered strategy: environment variable ($HOME / USERPROFILE /
// HOMEDRIVE+HOMEPATH), os/user.Current(), macOS dscl, and finally a shell
// fallback as a last resort.
//
// CGO=0 note: in CGO-disabled builds (e.g., Alpine/musl containers, distroless
// images, or builds with -tags netgo), the pure-Go user.Current() reads only
// /etc/passwd. UIDs managed by NSS/LDAP/SSSD that are absent from /etc/passwd
// will not be found there; the shell fallback (getHomeFromShell) is the last
// resort for those environments. When the user is not in any password database,
// the shell fallback returns ErrBlankOutput.
package homedir

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DisableCache will disable caching of the home directory. Caching is enabled
// by default.
var DisableCache bool

var (
	homedirCache           string
	cacheLock              sync.RWMutex
	ErrCannotExpandHomeDir = errors.New("cannot expand user-specific home dir")
	ErrBlankOutput         = errors.New("blank output when reading home directory")
	ErrHomeDrivePathBlank  = errors.New("HOMEDRIVE, HOMEPATH, or USERPROFILE are blank")
	// ErrShellUnavailable is returned when the shell (sh) binary cannot be found.
	// This can happen in minimal containers (e.g., distroless) that lack a shell.
	ErrShellUnavailable = errors.New("shell (sh) not available")
	// ErrIDUnavailable is returned when id is not found and USER env var is empty.
	ErrIDUnavailable = errors.New("id binary not found and USER env var is empty")

	// usernameRe is a whitelist for valid Unix usernames. It accepts only
	// letters, digits, dots, underscores, and hyphens. All other characters
	// (including shell operators |, &, >, <, (, ), tab, newline, space,
	// single quotes, backticks, $, ;, /) are implicitly rejected, preventing
	// shell injection when the username is interpolated into sh -c commands.
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	// externalCmdTimeout is the maximum time allowed for external subprocess
	// calls (id, dscl, sh). A conservative timeout prevents Dir()/Expand()
	// from hanging indefinitely on slow NSS/LDAP/directory backends.
	externalCmdTimeout = 5 * time.Second
)

// currentUserFunc is the function used to look up the current OS user.
// It is a variable so that tests can replace it with a stub to simulate
// error conditions without needing OS-level changes.
var currentUserFunc = user.Current

// darwinHomeDirFunc is the function used to look up the home directory on
// macOS via dscl. It accepts a cached username (may be empty) from dirUnix so
// that currentUserFunc is not called a second time on the darwin fallback path.
// It is a variable so that tests can replace it with a stub to exercise the
// darwin path on other operating systems.
var darwinHomeDirFunc = getDarwinHomeDir

// shellHomeDirFunc is the function used to get the home directory via the shell
// fallback when $HOME is unset/empty and user.Current() has failed.
// It can be replaced in tests to simulate failure or empty-output conditions.
//
// The default implementation (shellHomeDir) is injection-safe: it fetches the
// current username via os/exec, validates it, and then uses ~username tilde
// expansion (NOT eval with $HOME) so that the lookup is always backed by the
// system password database — independent of $HOME — and works in both bash
// and dash.
var shellHomeDirFunc = shellHomeDir

// Dir returns the home directory for the executing user.
//
// This uses an OS-specific method for discovering the home directory.
// An error is returned if a home directory cannot be detected.
//
// Thread safety: Dir is safe for concurrent use. The DisableCache variable is
// read under cacheLock to avoid data races. However, writing DisableCache from
// multiple goroutines concurrently is still unsafe — it must be set before any
// concurrent calls to Dir.
func Dir() (string, error) {
	cacheLock.RLock()
	disableCache := DisableCache
	cached := homedirCache
	cacheLock.RUnlock()

	if !disableCache && cached != "" {
		return cached, nil
	}

	cacheLock.Lock()
	defer cacheLock.Unlock()

	// Re-check after acquiring the write lock: another goroutine may have
	// already populated the cache between our read-unlock and write-lock.
	// Re-read DisableCache under the write lock for a consistent view.
	disableCache = DisableCache
	if !disableCache && homedirCache != "" {
		return homedirCache, nil
	}

	var result string
	var err error
	if runtime.GOOS == "windows" {
		result, err = dirWindows()
	} else {
		// Unix-like system, so just assume Unix
		result, err = dirUnix()
	}

	if err != nil {
		return "", err
	}
	// Only write to the cache when caching is enabled. If DisableCache is true,
	// skip the write to prevent a stale entry from poisoning the cache for
	// future callers that have caching enabled.
	if !disableCache {
		homedirCache = result
	}
	return result, nil
}

// Expand expands the path to include the home directory if the path
// is prefixed with `~`. If it isn't prefixed with `~`, the path is
// returned as-is.
func Expand(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return path, nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", ErrCannotExpandHomeDir
	}

	dir, err := Dir()
	if err != nil {
		return "", err
	}

	// Strip any leading path separator from path[1:] so that filepath.Join
	// correctly anchors the result under dir on all platforms. On Windows a
	// leading slash makes the path drive-relative, which would discard dir.
	rest := strings.TrimLeft(path[1:], `/\`)
	return filepath.Join(dir, rest), nil
}

// Reset clears the cache, forcing the next call to Dir to re-detect
// the home directory. This generally never has to be called, but can be
// useful in tests if you're modifying the home directory via the HOME
// env var or something.
func Reset() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	homedirCache = ""
}

func dirUnix() (string, error) {
	// Try to get the home directory from the environment variable first.
	// Apply filepath.Clean for consistency with dirWindows() which also
	// normalises all returned paths.
	if home := getHomeFromEnv(); home != "" {
		return filepath.Clean(home), nil
	}

	// Try user.Current() on all platforms first — it is the most reliable
	// and avoids spawning subprocesses. The darwin-specific dscl lookup is
	// kept as a fallback for environments where user.Current() fails.
	//
	// Note: in CGO=0 builds (e.g., Alpine/musl), user.Current() uses a
	// pure-Go implementation that reads only /etc/passwd. Users whose UID is
	// managed by NSS/LDAP/SSSD and not listed in /etc/passwd will not be found
	// here; the shell fallback below is the last resort for those environments.
	//
	// The user object is retained so that getDarwinHomeDir can reuse the
	// Username without a second currentUserFunc() call on the darwin path.
	var cachedUsername string
	if u, err := currentUserFunc(); err == nil {
		if u.HomeDir != "" {
			return u.HomeDir, nil
		}
		// HomeDir is empty but Username may still be available for dscl.
		cachedUsername = u.Username
	}

	// Darwin-specific dscl fallback (only reached when user.Current() fails
	// or returns an empty HomeDir). Pass the cached username to avoid a
	// second currentUserFunc() call inside darwinHomeDirFunc.
	if runtime.GOOS == "darwin" {
		if home, err := darwinHomeDirFunc(cachedUsername); err == nil && home != "" {
			return home, nil
		}
	}

	// Fallback to shell command
	return getHomeFromShell()
}

func getHomeFromEnv() string {
	homeEnv := "HOME"
	if runtime.GOOS == "plan9" {
		homeEnv = "home" // On Plan 9, env vars are lowercase
	}
	// TrimSpace for consistency with dirWindows(), which also trims env vars.
	// A whitespace-only value is treated the same as an empty value.
	return strings.TrimSpace(os.Getenv(homeEnv))
}

func dirWindows() (string, error) {
	// First prefer the HOME environmental variable.
	// Apply filepath.FromSlash before filepath.Clean so that forward-slash
	// paths set by Cygwin, Git Bash, or WSL1 (e.g., "/home/user") are
	// converted to native Windows separators. Note: POSIX-style absolute
	// paths become drive-relative after the separator conversion on Windows
	// (e.g., "/home/user" → "\home\user"). Callers that need a guaranteed
	// absolute path should set HOME to a drive-absolute value (e.g.,
	// "C:\Users\user").
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return cleanWindowsPath(home), nil
	}

	// Prefer standard environment variable USERPROFILE
	if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
		return cleanWindowsPath(home), nil
	}

	drive := strings.TrimSpace(os.Getenv("HOMEDRIVE"))
	path := strings.TrimSpace(os.Getenv("HOMEPATH"))
	if drive == "" || path == "" {
		return "", ErrHomeDrivePathBlank
	}

	// HOMEPATH should have a leading backslash (e.g., \Users\foo).
	// Enforce it to prevent a drive-relative path (e.g., C:Users\foo)
	// when an unconventional value like "Users\foo" is set.
	if !strings.HasPrefix(path, "\\") {
		path = "\\" + path
	}

	return filepath.Clean(drive + path), nil
}

// cleanWindowsPath converts forward slashes to the native Windows separator and
// applies filepath.Clean. This prevents drive-relative paths (e.g., \home\user)
// when tools like Cygwin or Git Bash set HOME with POSIX-style forward slashes.
func cleanWindowsPath(path string) string {
	return filepath.Clean(filepath.FromSlash(path))
}

// getDarwinHomeDir looks up the home directory via macOS dscl.
// cachedUsername may be non-empty when dirUnix already called currentUserFunc
// and obtained the username; passing it here avoids a second OS user lookup.
// When cachedUsername is empty, getDarwinHomeDir falls back to id -un.
func getDarwinHomeDir(cachedUsername string) (string, error) {
	var username string
	if cachedUsername != "" {
		// Reuse the username from the dirUnix caller — no second currentUserFunc call.
		username = cachedUsername
	} else {
		// Fall back to id -un when no cached username is available (e.g., when
		// called directly from tests via darwinHomeDirFunc).
		var whoOut, whoErr bytes.Buffer
		idCtx, idCancel := context.WithTimeout(context.Background(), externalCmdTimeout)
		defer idCancel()
		whoCmd := exec.CommandContext(idCtx, "id", "-un")
		whoCmd.Stdout = &whoOut
		whoCmd.Stderr = &whoErr
		if err := whoCmd.Run(); err != nil {
			msg := strings.TrimSpace(whoErr.String())
			if msg != "" {
				return "", fmt.Errorf("getDarwinHomeDir: id: %w (stderr: %s)", err, msg)
			}
			return "", fmt.Errorf("getDarwinHomeDir: id: %w", err)
		}
		username = strings.TrimSpace(whoOut.String())
		if username == "" {
			return "", ErrBlankOutput
		}
	}

	// Guard against path traversal and shell injection: dscl path is
	// /Users/<username>. Use a strict whitelist that accepts only letters,
	// digits, dots, underscores, and hyphens. This implicitly rejects path
	// separators, control characters, shell operators (|, &, >, <, (, )),
	// quotes, backticks, dollar signs, semicolons, spaces, and tabs.
	if !usernameRe.MatchString(username) {
		return "", fmt.Errorf("getDarwinHomeDir: invalid characters in username %q", username)
	}

	// Query the directory service without a shell pipeline or sed.
	var out, dsErr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), externalCmdTimeout)
	defer cancel()
	dsCmd := exec.CommandContext(ctx, "dscl", "-q", ".", "-read", "/Users/"+username, "NFSHomeDirectory")
	dsCmd.Stdout = &out
	dsCmd.Stderr = &dsErr
	if err := dsCmd.Run(); err != nil {
		msg := strings.TrimSpace(dsErr.String())
		if msg != "" {
			return "", fmt.Errorf("getDarwinHomeDir: dscl: %w (stderr: %s)", err, msg)
		}
		return "", fmt.Errorf("getDarwinHomeDir: dscl: %w", err)
	}

	// dscl output format: "NFSHomeDirectory: /Users/username"
	for _, line := range strings.Split(out.String(), "\n") {
		if after, ok := strings.CutPrefix(line, "NFSHomeDirectory:"); ok {
			home := filepath.Clean(strings.TrimSpace(after))
			if home != "" && home != "." {
				return home, nil
			}
		}
	}
	return "", ErrBlankOutput
}

func getUnixHomeDir() (string, error) {
	// Use os/user.Current to get the home directory without parsing
	// /etc/passwd format data directly. This avoids handling raw password
	// database entries, which can contain sensitive fields.
	u, err := currentUserFunc()
	if err != nil {
		return "", fmt.Errorf("getUnixHomeDir: %w", err)
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("getUnixHomeDir: %w", ErrBlankOutput)
	}
	return u.HomeDir, nil
}

func getHomeFromShell() (string, error) {
	return shellHomeDirFunc()
}

// shellGetUsernameFunc is the function used by shellHomeDir to obtain the
// current username via id -un. If id is not found in $PATH, it falls back
// to the USER environment variable and then to whoami, so that minimal
// containers without id can still look up the home directory.
// It can be replaced in tests to simulate failure conditions.
var shellGetUsernameFunc = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), externalCmdTimeout)
	defer cancel()
	var idOut, idErr bytes.Buffer
	idCmd := exec.CommandContext(ctx, "id", "-un")
	idCmd.Stdout = &idOut
	idCmd.Stderr = &idErr
	if err := idCmd.Run(); err != nil {
		// If id is missing entirely, try $USER then whoami before giving up.
		if errors.Is(err, exec.ErrNotFound) {
			if u := strings.TrimSpace(os.Getenv("USER")); u != "" {
				return u, nil
			}
			whoamiCtx, whoamiCancel := context.WithTimeout(context.Background(), externalCmdTimeout)
			defer whoamiCancel()
			var whoOut, whoErr bytes.Buffer
			whoamiCmd := exec.CommandContext(whoamiCtx, "whoami")
			whoamiCmd.Stdout = &whoOut
			whoamiCmd.Stderr = &whoErr
			if werr := whoamiCmd.Run(); werr == nil {
				if name := strings.TrimSpace(whoOut.String()); name != "" {
					return name, nil
				}
			}
			return "", ErrIDUnavailable
		}
		msg := strings.TrimSpace(idErr.String())
		if msg != "" {
			return "", fmt.Errorf("getHomeFromShell: id: %w (stderr: %s)", err, msg)
		}
		return "", fmt.Errorf("getHomeFromShell: id: %w", err)
	}
	return strings.TrimSpace(idOut.String()), nil
}

// shellHomeDir is the production implementation of the shell home-directory
// fallback. It is injection-safe because it:
//  1. Fetches the current username via shellGetUsernameFunc (id -un by default).
//  2. Validates the username with the same guard used by getDarwinHomeDir.
//  3. Constructs a shell command that expands ~username (not bare ~) so that
//     the lookup is backed by the system password database in both bash and
//     dash, independent of $HOME.
//
// This replaces the previous `eval "echo ~$(id -un)"` which was vulnerable to
// shell metacharacter injection if id -un ever returned a malicious string.
func shellHomeDir() (string, error) {
	// Step 1: obtain the current username in Go, outside the shell.
	username, err := shellGetUsernameFunc()
	if err != nil {
		return "", err
	}
	if username == "" {
		return "", ErrBlankOutput
	}

	// Step 2: validate the username to prevent injection in the shell command.
	// Use a strict whitelist that accepts only letters, digits, dots,
	// underscores, and hyphens. This implicitly rejects path separators,
	// control characters, shell operators (|, &, >, <, (, )), quotes,
	// backticks, dollar signs, semicolons, spaces, and tabs.
	if !usernameRe.MatchString(username) {
		return "", fmt.Errorf("getHomeFromShell: invalid characters in username %q", username)
	}

	// Step 3: expand ~username in the shell.
	// printf '%s\n' ~username expands the home directory from the system
	// password database in both bash and dash, even when $HOME is unset.
	// Safe to interpolate: username is pre-validated above to contain only
	// safe characters (no shell metacharacters), making string concatenation
	// equivalent to passing a literal.
	shCtx, shCancel := context.WithTimeout(context.Background(), externalCmdTimeout)
	defer shCancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(shCtx, "sh", "-c", "printf '%s\\n' ~"+username)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// If sh is not available at all, return a sentinel error so callers
		// can distinguish "no shell" from "user not found" or "id failed".
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrShellUnavailable
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("getHomeFromShell: %w (stderr: %s)", err, msg)
		}
		return "", fmt.Errorf("getHomeFromShell: %w", err)
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", ErrBlankOutput
	}
	// Guard against unexpanded tilde (e.g., "~username") which occurs when the
	// user is not in the password database (distroless/scratch containers). A
	// tilde-prefixed string is not a valid absolute path and must not be returned
	// as a home directory.
	if strings.HasPrefix(result, "~") {
		return "", ErrBlankOutput
	}
	return result, nil
}
