package homedir

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
)

// currentUserFunc is the function used to look up the current OS user.
// It is a variable so that tests can replace it with a stub to simulate
// error conditions without needing OS-level changes.
var currentUserFunc = user.Current

// darwinHomeDirFunc is the function used to look up the home directory on
// macOS via dscl. It is a variable so that tests can replace it with a stub
// to exercise the darwin path on other operating systems.
var darwinHomeDirFunc = getDarwinHomeDir

// shellHomeDirCmd is the shell command used by getHomeFromShell to determine
// the home directory. It can be replaced in tests to simulate failure or
// empty-output conditions.
//
// The default uses `eval "echo ~$(id -un)"` which expands the tilde for the
// current user via the password database, without relying on $HOME. This is
// important because getHomeFromShell is called only when $HOME is unset or
// empty and user.Current() has also failed — using `cd && pwd` in that
// scenario would return the current working directory instead of the home
// directory.
var shellHomeDirCmd = `eval "echo ~$(id -un)"`

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
	if home, err := getUnixHomeDir(); err == nil && home != "" {
		return home, nil
	}

	// Darwin-specific dscl fallback (only reached when user.Current() fails).
	if runtime.GOOS == "darwin" {
		if home, err := darwinHomeDirFunc(); err == nil && home != "" {
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
	// First prefer the HOME environmental variable
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Clean(home), nil
	}

	// Prefer standard environment variable USERPROFILE
	if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
		return filepath.Clean(home), nil
	}

	drive := strings.TrimSpace(os.Getenv("HOMEDRIVE"))
	path := strings.TrimSpace(os.Getenv("HOMEPATH"))
	if drive == "" || path == "" {
		return "", ErrHomeDrivePathBlank
	}

	return filepath.Clean(drive + path), nil
}

func getDarwinHomeDir() (string, error) {
	// Prefer user.Current().Username to avoid spawning an extra subprocess.
	// user.Current() is called earlier in the chain (getUnixHomeDir) and may
	// have failed to populate HomeDir, but Username is often still available.
	// Cache the result to avoid calling currentUserFunc() twice (once here
	// and once in getUnixHomeDir which runs before getDarwinHomeDir).
	var username string
	cachedUser, userErr := currentUserFunc()
	if userErr == nil && cachedUser.Username != "" {
		username = cachedUser.Username
	} else {
		// Fall back to id -un when user.Current() fails or returns an empty username.
		var whoOut bytes.Buffer
		whoCmd := exec.Command("id", "-un")
		whoCmd.Stdout = &whoOut
		if err := whoCmd.Run(); err != nil {
			return "", fmt.Errorf("getDarwinHomeDir: id: %w", err)
		}
		username = strings.TrimSpace(whoOut.String())
		if username == "" {
			return "", ErrBlankOutput
		}
	}

	// Guard against path traversal: dscl path is /Users/<username>, so the
	// username must not contain path separators, null bytes, or newlines.
	if strings.ContainsAny(username, "/\\\x00\n\r") {
		return "", fmt.Errorf("getDarwinHomeDir: invalid characters in username %q", username)
	}

	// Query the directory service without a shell pipeline or sed.
	var out bytes.Buffer
	dsCmd := exec.Command("dscl", "-q", ".", "-read", "/Users/"+username, "NFSHomeDirectory")
	dsCmd.Stdout = &out
	if err := dsCmd.Run(); err != nil {
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
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", shellHomeDirCmd)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
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
