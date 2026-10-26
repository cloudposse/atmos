package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Uninstall removes Atmos-generated shims from .git/hooks. With names, only those hooks are
// removed; without names, every Atmos shim in the hooks directory is removed, including orphans
// whose hook is no longer configured (they would otherwise fail every Git operation with
// "git hook not configured"). User-authored hooks are never deleted; a warning is emitted instead.
func Uninstall(ctx context.Context, cfg *schema.GitConfig, names []string) error {
	defer perf.Track(nil, "hooks.Uninstall")()

	hooksDir, err := resolveHooksDir(ctx)
	if err != nil {
		return err
	}

	if len(names) == 0 {
		return uninstallAll(hooksDir)
	}

	for _, name := range names {
		if err := checkUninstallTarget(hooksDir, name, cfg); err != nil {
			return err
		}
	}
	for _, name := range names {
		if err := uninstallHook(hooksDir, name); err != nil {
			return err
		}
	}

	return nil
}

// checkUninstallTarget rejects a name that is neither configured nor an existing Atmos shim.
// An orphan shim for an unconfigured hook stays removable by name.
func checkUninstallTarget(hooksDir, name string, cfg *schema.GitConfig) error {
	if err := ValidateShimName(name); err != nil {
		return err
	}
	if cfg != nil {
		if _, ok := cfg.Hooks[name]; ok {
			return nil
		}
	}
	if isAtmosShim(filepath.Join(hooksDir, name)) {
		return nil
	}
	var configured map[string]schema.GitHookEntry
	if cfg != nil {
		configured = cfg.Hooks
	}
	return NotConfiguredError(name, configured)
}

// uninstallAll removes every Atmos-generated shim found in hooksDir.
func uninstallAll(hooksDir string) error {
	entries, err := os.ReadDir(hooksDir)
	if os.IsNotExist(err) {
		ui.Info("No Atmos-managed Git hook shims found.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading hooks directory %q: %w", hooksDir, err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || ValidateShimName(entry.Name()) != nil || !isAtmosShim(filepath.Join(hooksDir, entry.Name())) {
			continue
		}
		if err := uninstallHook(hooksDir, entry.Name()); err != nil {
			return err
		}
		removed++
	}
	if removed == 0 {
		ui.Info("No Atmos-managed Git hook shims found.")
	}

	return nil
}

// isAtmosShim reports whether path is a regular file carrying the Atmos shim marker.
func isAtmosShim(path string) bool {
	content, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(content), ShimMarker)
}

// uninstallHook removes the shim for hookName from hooksDir, only if it is
// an Atmos-generated file. User-authored hooks are never deleted.
func uninstallHook(hooksDir, hookName string) error {
	if err := ValidateShimName(hookName); err != nil {
		return err
	}

	dest := filepath.Join(hooksDir, hookName)

	content, err := os.ReadFile(dest)
	if os.IsNotExist(err) {
		// Already gone; treat as a no-op.
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading hook file %q: %w", dest, err)
	}

	if !strings.Contains(string(content), ShimMarker) {
		ui.Warningf(
			"Skipping %q: not an Atmos-generated shim (marker not found). Remove it manually if needed.",
			dest,
		)
		return nil
	}

	if err := os.Remove(dest); err != nil {
		return fmt.Errorf("removing hook shim %q: %w", dest, err)
	}

	ui.Successf("Removed hook shim: %s", dest)
	return nil
}
