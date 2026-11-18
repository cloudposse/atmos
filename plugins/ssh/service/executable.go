package service

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

const (
	stablePrefix = "atmos-ssh-"
	stableMode   = os.FileMode(0o755)
)

// versionRE limits versions to characters that are safe in a file name.
var versionRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// ErrInvalidVersion means a version cannot be used in an executable file name.
var ErrInvalidVersion = errors.New("invalid version")

// StablePath returns where the copy of the executable for version lives: a file in the plugin's
// bin directory whose name carries the version. Unlike the installed plugin path, it does not
// change when the plugin updates, so a LaunchAgent can point at it.
func StablePath(p *paths.Paths, version string) (string, error) {
	if !versionRE.MatchString(version) {
		return "", fmt.Errorf("%w: %q", ErrInvalidVersion, version)
	}
	name := stablePrefix + version
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(p.BinDir(), name), nil
}

// ParseStableVersion extracts the version from a path produced by StablePath.
func ParseStableVersion(exe string) (string, bool) {
	base := strings.TrimSuffix(filepath.Base(exe), ".exe")
	version, ok := strings.CutPrefix(base, stablePrefix)
	if !ok || !versionRE.MatchString(version) {
		return "", false
	}
	return version, true
}

// InstallExecutable copies the running executable to its stable, versioned path and returns that
// path.
func InstallExecutable(p *paths.Paths, version string) (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the running executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(src); err == nil {
		src = resolved
	}
	return CopyExecutable(p, src, version)
}

// CopyExecutable copies src to the stable path for version, atomically and with mode 0755. A copy
// that is already there with the same content is left alone.
func CopyExecutable(p *paths.Paths, src, version string) (string, error) {
	dst, err := StablePath(p, version)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read executable %s: %w", src, err)
	}
	if same, _ := sameContent(dst, data); same {
		return dst, ensureMode(dst)
	}
	if err := os.MkdirAll(p.BinDir(), paths.DirMode); err != nil {
		return "", fmt.Errorf("create %s: %w", p.BinDir(), err)
	}
	if err := writeFileAtomic(dst, data, stableMode); err != nil {
		return "", err
	}
	return dst, nil
}

// sameContent reports whether the file at path holds exactly data.
func sameContent(path string, data []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	want := sha256.Sum256(data)
	return bytes.Equal(h.Sum(nil), want[:]), nil
}

func ensureMode(path string) error {
	if err := os.Chmod(path, stableMode); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("set mode on %s: %w", path, err)
	}
	return nil
}
