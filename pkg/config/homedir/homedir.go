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
var shellHomeDirCmd = "cd && pwd"

// Dir returns the home directory for the executing user.
//
// This uses an OS-specific method for discovering the home directory.
// An error is returned if a home directory cannot be detected.
func Dir() (string, error) {
	if !DisableCache {
		cacheLock.RLock()
		cached := homedirCache
		cacheLock.RUnlock()
		if cached != "" {
			return cached, nil
		}
	}

	cacheLock.Lock()
	defer cacheLock.Unlock()

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
	homedirCache = result
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
	// Try to get the home directory from the environment variable first
	if home := getHomeFromEnv(); home != "" {
		return home, nil
	}

	// Try user.Current() on all platforms first — it is the most reliable
	// and avoids spawning subprocesses. The darwin-specific dscl lookup is
	// kept as a fallback for environments where user.Current() fails.
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
	return os.Getenv(homeEnv)
}

func dirWindows() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Clean(home), nil
	}

	// Prefer standard environment variable USERPROFILE
	if home := os.Getenv("USERPROFILE"); home != "" {
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
	// Obtain the current username without a shell to avoid quoting and
	// injection risks.
	var whoOut bytes.Buffer
	whoCmd := exec.Command("id", "-un")
	whoCmd.Stdout = &whoOut
	if err := whoCmd.Run(); err != nil {
		return "", fmt.Errorf("getDarwinHomeDir: id: %w", err)
	}
	username := strings.TrimSpace(whoOut.String())
	if username == "" {
		return "", ErrBlankOutput
	}
	// Guard against path traversal: dscl path is /Users/<username>, so the
	// username must not contain path separators or null bytes.
	if strings.ContainsAny(username, "/\\\x00") {
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
	return u.HomeDir, nil
}

func getHomeFromShell() (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", shellHomeDirCmd)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", ErrBlankOutput
	}
	return result, nil
}
