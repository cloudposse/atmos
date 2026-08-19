//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magefile/mage/sh"
)

// Lintroller builds (if stale) and runs the standalone Atmos lintroller
// analyzer binary against every non-testdata package.
func (Lint) Lintroller() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "tools", "lintroller")
	binPath := filepath.Join(dir, lintrollerBinaryName())

	stale, err := lintrollerIsStale(dir, binPath)
	if err != nil {
		return err
	}
	if stale {
		fmt.Println("Building lintroller...")
		if buildErr := sh.RunWith(map[string]string{"GOTOOLCHAIN": "auto"},
			"go", "build", "-C", dir, "-o", filepath.Base(binPath), "./cmd/lintroller"); buildErr != nil {
			return fmt.Errorf("mage: build lintroller: %w", buildErr)
		}
		if chmodErr := os.Chmod(binPath, dirPerm); chmodErr != nil {
			return fmt.Errorf("mage: chmod lintroller: %w", chmodErr)
		}
	}

	pkgsOut, err := sh.Output("go", "list", "-C", root, "./...")
	if err != nil {
		return fmt.Errorf("mage: go list: %w", err)
	}
	pkgs := filterOutTestdata(strings.Split(pkgsOut, "\n"))

	fmt.Println("Running lintroller (Atmos custom rules)...")
	return sh.RunV(binPath, pkgs...)
}

// lintrollerBinaryName returns the platform-appropriate name for the
// standalone lintroller binary (mirrors tools/lintroller/.lintroller from
// the previous bash step).
func lintrollerBinaryName() string {
	name := ".lintroller"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// lintrollerIsStale reports whether the binary is missing or older than any
// *.go, go.mod, or go.sum file under dir.
func lintrollerIsStale(dir, binPath string) (bool, error) {
	info, err := os.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("mage: stat %s: %w", binPath, err)
	}
	return dirHasFileNewerThan(dir, info.ModTime(), isGoModuleFile)
}

// filterOutTestdata drops empty entries and any package under a testdata
// directory, mirroring `go list ./... | grep -v '/testdata'`.
func filterOutTestdata(pkgs []string) []string {
	out := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if pkg == "" || strings.Contains(pkg, "/testdata") {
			continue
		}
		out = append(out, pkg)
	}
	return out
}
