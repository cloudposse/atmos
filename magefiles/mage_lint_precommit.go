//go:build mage

package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
)

// Precommit runs golangci-lint (with lintroller) against changed files using
// the pre-built ./custom-gcl binary.
//
// IMPORTANT: this target must never build ./custom-gcl itself. Building
// `golangci-lint custom` (a full clone + recompile of golangci-lint) inside a
// pre-commit hook previously corrupted git worktrees; the fix is that the
// hook only ever checks for a pre-built binary and fails fast with a helpful
// message if it's missing. Do not "helpfully" wire CustomGCL in as a
// dependency here.
func (Lint) Precommit() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}
	binPath := customGCLBinaryPath(root)
	stale, err := customGCLIsStale(root, binPath)
	if err != nil {
		return err
	}
	if stale {
		fmt.Fprintln(os.Stderr, "Error: custom-gcl binary is missing or stale.")
		fmt.Fprintln(os.Stderr, "Please build it first by running: go tool mage lint:customGCL")
		return mg.Fatal(1, "custom-gcl binary is missing or stale")
	}
	return runGolangciLintPrecommit(root, binPath)
}
