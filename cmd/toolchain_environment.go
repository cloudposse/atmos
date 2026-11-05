package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

// installedProjectPathEnv tracks only directories inserted by shared startup.
// It is inherited process bookkeeping, not a user-facing configuration option.
const installedProjectPathEnv = "_ATMOS_TOOLCHAIN_PATH"

// applyInstalledProjectTools exposes the project baseline at the shared command
// boundary. Scoped dependency environments prepend their overrides later.
func applyInstalledProjectTools(atmosConfig *schema.AtmosConfiguration) error {
	// A nested Atmos invocation may select a different project. Clear the previous
	// baseline even when the new manifest is missing, unreadable, or invalid.
	if err := replaceInstalledProjectPath(nil); err != nil {
		return err
	}
	env, err := dependencies.ForInstalledProjectTools(atmosConfig)
	if err != nil {
		return err
	}
	return replaceInstalledProjectPath(env.ToolchainDirs())
}

// replaceInstalledProjectPath replaces Atmos's baseline while preserving user-supplied PATH entries.
func replaceInstalledProjectPath(dirs []string) error {
	previous, _ := os.LookupEnv(installedProjectPathEnv)
	if len(dirs) == 0 && previous == "" {
		return nil
	}

	// PATH and its ownership marker belong to the inherited process environment.
	currentPath, _ := os.LookupEnv("PATH")
	previousDirs := filepath.SplitList(previous)
	var inherited []string
	for _, dir := range filepath.SplitList(currentPath) {
		if !slices.Contains(previousDirs, dir) {
			inherited = append(inherited, dir)
		}
	}

	// Keep repeated initialization idempotent, including when proxies are also
	// prepended to PATH. Preserve the order of unrelated inherited entries.
	selected := make(map[string]bool, len(dirs))
	var added []string
	for _, dir := range dirs {
		selected[dir] = true
		// A directory already present in the user's PATH must survive cleanup.
		if !slices.Contains(inherited, dir) {
			added = append(added, dir)
		}
	}
	result := slices.Clone(dirs)
	for _, dir := range inherited {
		if !selected[dir] {
			result = append(result, dir)
		}
	}
	if err := os.Setenv("PATH", strings.Join(result, string(os.PathListSeparator))); err != nil {
		return err
	}
	return os.Setenv(installedProjectPathEnv, strings.Join(added, string(os.PathListSeparator)))
}
