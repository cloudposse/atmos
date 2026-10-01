//go:build mage

// Package main defines this repository's Mage targets for golangci-lint
// custom-build orchestration (customgcl, lintroller, gomodcheck, precommit,
// changed). These files are excluded from the normal `go build ./...` set by
// the mage build tag; they are only compiled when invoked through `go tool mage`.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Lint groups golangci-lint custom-build orchestration targets.
type Lint mg.Namespace

// Default is the target `go tool mage` runs when invoked with no arguments.
var Default = Lint.Changed

var errMageRepoRootNotFound = errors.New("mage: could not locate repository root (go.mod for module github.com/cloudposse/atmos)")

// rootModuleDecl is the expected first line of the root module's go.mod,
// used by mageRepoRoot to recognize the repository root.
const rootModuleDecl = "module github.com/cloudposse/atmos"

// mageRepoRoot walks up from the working directory looking for the go.mod
// that declares this module, so targets behave the same regardless of the
// directory they're invoked from.
func mageRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("mage: getwd: %w", err)
	}
	for {
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			firstLine, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(firstLine) == rootModuleDecl {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errMageRepoRootNotFound
		}
		dir = parent
	}
}

// mageGoVersion returns `go env GOVERSION` verbatim (e.g. "go1.26.4").
func mageGoVersion() (string, error) {
	version, err := sh.Output("go", "env", "GOVERSION")
	if err != nil {
		return "", fmt.Errorf("mage: go env GOVERSION: %w", err)
	}
	return version, nil
}

// isGoSourceFile reports whether name is a Go source file.
func isGoSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

// isGoModuleFile reports whether name is a Go source file or a module
// manifest (go.mod/go.sum).
func isGoModuleFile(name string) bool {
	return isGoSourceFile(name) || name == "go.mod" || name == "go.sum"
}

// dirHasFileNewerThan reports whether dir contains a regular file, accepted
// by match, whose modification time is after cutoff. It is the cross-platform
// replacement for `find <dir> -type f ... -newer <ref> -print -quit`.
func dirHasFileNewerThan(dir string, cutoff time.Time, match func(name string) bool) (bool, error) {
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
