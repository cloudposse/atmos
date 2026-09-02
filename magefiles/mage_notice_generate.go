//go:build mage

package main

import (
	"fmt"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Notice groups NOTICE-file generation targets.
type Notice mg.Namespace

// Generate regenerates NOTICE from Go dependency licenses via
// tools/noticegen (which wraps go-licenses), replacing the previous
// scripts/generate-notice.sh. It's a separate module (like
// tools/gomodcheck) rather than a package imported into magefiles, so its
// dependencies never need a `replace` directive in the root go.mod -- which
// Lint.GoModCheck forbids.
func (Notice) Generate() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}
	toolDir := filepath.Join(root, "tools", "noticegen")
	noticePath := filepath.Join(root, "NOTICE")
	if runErr := sh.RunWithV(map[string]string{"GOTOOLCHAIN": "auto"},
		"go", "run", "-C", toolDir, ".", root, noticePath); runErr != nil {
		return fmt.Errorf("mage: notice generate: %w", runErr)
	}
	return nil
}
