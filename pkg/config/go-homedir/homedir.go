package homedir

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

const (
	passwdFieldCount   = 7
	passwdHomeDirIndex = 5
)

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

	return filepath.Join(dir, path[1:]), nil
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

	// Try OS-specific methods
	if runtime.GOOS == "darwin" {
		if home, err := getDarwinHomeDir(); err == nil && home != "" {
			return home, nil
		}
	} else {
		if home, err := getUnixHomeDir(); err == nil && home != "" {
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
		return home, nil
	}

	// Prefer standard environment variable USERPROFILE
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home, nil
	}

	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		return "", ErrHomeDrivePathBlank
	}

	return home, nil
}

func getDarwinHomeDir() (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", `dscl -q . -read /Users/"$(whoami)" NFSHomeDirectory | sed 's/^[^ ]*: //'`)
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

func getUnixHomeDir() (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("getent", "passwd", strconv.Itoa(os.Getuid()))
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return "", err
		}
		return "", nil
	}

	passwd := strings.TrimSpace(stdout.String())
	if passwd == "" {
		return "", nil
	}

	// username:password:uid:gid:gecos:home:shell
	// lgtm[go/clear-text-logging]
	// The password field in modern /etc/passwd is 'x', not the actual password.
	// We only extract the home directory field and do not log sensitive data.
	passwdParts := strings.SplitN(passwd, ":", passwdFieldCount)
	if len(passwdParts) > passwdHomeDirIndex {
		return passwdParts[passwdHomeDirIndex], nil
	}
	return "", nil
}

func getHomeFromShell() (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", "cd && pwd")
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
