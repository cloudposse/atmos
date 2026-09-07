//go:build mage

package main

import (
	"fmt"
	"path/filepath"

	"github.com/magefile/mage/sh"
)

// GoModCheck verifies go.mod has no replace/exclude directives that would
// break `go install` for consumers of this module. It runs via `go run`
// rather than a staleness-cached binary — Go's build cache already makes
// repeat compiles fast, so the hand-rolled cached-binary staleness check the
// atmos-command version previously had (mirroring tools/lintroller's) bought
// little for the extra code, and this now gives the pre-commit hook and
// `atmos lint gomodcheck` byte-identical behavior for the first time.
func (Lint) GoModCheck() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}
	goModPath := filepath.Join(root, "go.mod")
	toolDir := filepath.Join(root, "tools", "gomodcheck")
	if runErr := sh.RunWithV(map[string]string{"GOTOOLCHAIN": "auto"},
		"go", "run", "-C", toolDir, ".", goModPath); runErr != nil {
		return fmt.Errorf("mage: gomodcheck: %w", runErr)
	}
	return nil
}
