package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

// applyInstalledProjectTools exposes the project baseline at the shared command
// boundary. Scoped dependency environments prepend their overrides later.
func applyInstalledProjectTools(atmosConfig *schema.AtmosConfiguration) error {
	env, err := dependencies.ForInstalledProjectTools(atmosConfig)
	if err != nil {
		return err
	}
	dirs := env.ToolchainDirs()
	if len(dirs) == 0 {
		return nil
	}

	// Keep repeated initialization idempotent, including when proxies are also
	// prepended to PATH. Preserve the order of unrelated inherited entries.
	selected := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		selected[dir] = true
	}
	// PATH belongs to the inherited process environment, not Atmos configuration.
	currentPath, _ := os.LookupEnv("PATH")
	for _, dir := range filepath.SplitList(currentPath) {
		if !selected[dir] {
			dirs = append(dirs, dir)
		}
	}
	return os.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator)))
}
