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

// Uninstall removes Atmos-generated shims from .git/hooks for the named hooks
// (all configured hooks when names is empty). User-authored hooks are never
// deleted; a warning is emitted instead.
func Uninstall(ctx context.Context, cfg *schema.GitConfig, names []string) error {
	defer perf.Track(nil, "hooks.Uninstall")()

	hooksDir, err := resolveHooksDir(ctx)
	if err != nil {
		return err
	}

	hookNames := hookNamesOrConfigured(names, cfg)
	if len(hookNames) == 0 {
		ui.Info("No hooks configured under git.hooks in atmos.yaml.")
		return nil
	}

	for _, name := range hookNames {
		if err := uninstallHook(hooksDir, name); err != nil {
			return err
		}
	}

	return nil
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
