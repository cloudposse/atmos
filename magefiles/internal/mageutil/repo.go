package mageutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/sh"
)

var ErrRepoRootNotFound = errors.New("mage: could not locate repository root (go.mod for module github.com/cloudposse/atmos)")

// RootModuleDecl is the expected first line of the root module's go.mod,
// used by RepoRoot to recognize the repository root.
const RootModuleDecl = "module github.com/cloudposse/atmos"

// RepoRoot walks up from the working directory looking for the go.mod
// that declares this module, so targets behave the same regardless of the
// directory they're invoked from.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("mage: getwd: %w", err)
	}
	for {
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			firstLine, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(firstLine) == RootModuleDecl {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrRepoRootNotFound
		}
		dir = parent
	}
}

// GoVersion returns `go env GOVERSION` verbatim (e.g. "go1.26.4").
func GoVersion() (string, error) {
	version, err := sh.Output("go", "env", "GOVERSION")
	if err != nil {
		return "", fmt.Errorf("mage: go env GOVERSION: %w", err)
	}
	return version, nil
}

// IsGoSourceFile reports whether name is a Go source file.
func IsGoSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

// IsGoModuleFile reports whether name is a Go source file or a module
// manifest (go.mod/go.sum).
func IsGoModuleFile(name string) bool {
	return IsGoSourceFile(name) || name == "go.mod" || name == "go.sum"
}

// DirHasFileNewerThan reports whether dir contains a regular file, accepted
// by match, whose modification time is after cutoff. It is the cross-platform
// replacement for `find <dir> -type f ... -newer <ref> -print -quit`.
func DirHasFileNewerThan(dir string, cutoff time.Time, match func(name string) bool) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if found {
			return filepath.SkipAll
		}
		if d.IsDir() || !match(d.Name()) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.ModTime().After(cutoff) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("mage: walk %s: %w", dir, err)
	}
	return found, nil
}

// RunIn runs name with args in dir, with extraEnv appended on top of the
// current process environment (so later duplicate keys win), streaming
// output directly to this process's stdout/stderr.
func RunIn(dir string, extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...) // #nosec G204 -- fixed command names, mage-target-controlled args.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mage: run %s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
