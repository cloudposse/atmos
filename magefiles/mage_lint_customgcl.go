//go:build mage

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/magefile/mage/sh"
)

var errCustomGCLBuildFailed = errors.New("mage: golangci-lint custom failed")

// CustomGCL builds the custom golangci-lint binary (with the lintroller
// plugin compiled in) via `golangci-lint custom`, but only when the existing
// binary is missing or older than its build inputs (.custom-gcl.yml or any
// tools/lintroller source/module file). Rebuilding unconditionally is the
// single biggest waste in `lint changed` (golangci-lint custom clones and
// recompiles a full binary), so this mirrors the previous bash staleness
// guard exactly.
func (Lint) CustomGCL() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}
	binPath := customGCLBinaryPath(root)

	stale, err := customGCLIsStale(root, binPath)
	if err != nil {
		return err
	}
	if !stale {
		fmt.Println("custom-gcl is up to date; skipping rebuild.")
		return nil
	}

	fmt.Println("Building custom golangci-lint binary with lintroller plugin...")
	goVersion, err := mageGoVersion()
	if err != nil {
		return err
	}
	env := map[string]string{
		"GOFLAGS":     "-buildvcs=false",
		"GOTOOLCHAIN": goVersion,
	}
	if runErr := sh.RunWithV(env, "golangci-lint", "custom"); runErr != nil {
		return fmt.Errorf("%w: %w", errCustomGCLBuildFailed, runErr)
	}
	fmt.Println("Custom golangci-lint binary built successfully:", binPath)
	return nil
}

// customGCLBinaryPath returns the expected path of the built custom-gcl
// binary for the current platform. The .exe suffix on Windows matches Go's
// own `go build` convention, which `golangci-lint custom` builds on top of;
// this hasn't been verified against a real Windows run of `golangci-lint
// custom` (this repo's CI only builds/lints on Linux today).
func customGCLBinaryPath(root string) string {
	name := "custom-gcl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, name)
}

// customGCLIsStale mirrors the previous bash rule: rebuild if the binary is
// missing, or .custom-gcl.yml is newer than it, or any *.go/go.mod/go.sum
// file under tools/lintroller is newer than it.
func customGCLIsStale(root, binPath string) (bool, error) {
	info, err := os.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("mage: stat %s: %w", binPath, err)
	}
	binModTime := info.ModTime()

	cfgInfo, err := os.Stat(filepath.Join(root, ".custom-gcl.yml"))
	if err != nil {
		return false, fmt.Errorf("mage: stat .custom-gcl.yml: %w", err)
	}
	if cfgInfo.ModTime().After(binModTime) {
		return true, nil
	}

	return dirHasFileNewerThan(filepath.Join(root, "tools", "lintroller"), binModTime, isGoModuleFile)
}
