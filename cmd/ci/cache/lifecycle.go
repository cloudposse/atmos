package cache

import (
	"context"
	"sync"

	"github.com/spf13/cobra"

	cipkg "github.com/cloudposse/atmos/pkg/ci"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// pendingSave holds the manager registered for automatic save-on-exit.
// It is set during AutoRestore (when ci.cache.auto includes "save") and invoked
// from RunPendingSave (wired into the CLI Cleanup path) so the save runs on
// normal exit and on SIGINT/SIGTERM.
var (
	pendingSaveMu sync.Mutex
	pendingSave   *cachepkg.Manager
)

// AutoRestore performs automatic restore-on-start when the CI cache is enabled
// with auto restore, a cache-capable CI provider is detected, and the current
// command is eligible. It also registers an automatic save-on-exit when auto
// save is configured. All paths are no-ops (with a debug log) outside a
// supported CI provider, so this is safe to call for every invocation.
func AutoRestore(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "cache.AutoRestore")()

	// The explicit `atmos ci cache` subcommands manage the lifecycle themselves,
	// so skip the automatic hooks for them to avoid redundant work.
	if atmosConfig == nil || !atmosConfig.CI.Cache.Enabled || isCICacheCommand(cmd) {
		return
	}

	cfg, err := cachepkg.ResolveConfig(atmosConfig)
	if err != nil {
		log.Debug("CI cache: failed to resolve configuration", "error", err)
		return
	}
	if !cfg.AutoRestoreEnabled() && !cfg.AutoSaveEnabled() {
		return
	}

	backend, err := cipkg.DetectCache()
	if err != nil {
		log.Debug("CI cache: no cache-capable CI provider detected; skipping auto cache", "error", err)
		return
	}

	manager := cachepkg.NewManager(backend, cfg)
	if cfg.AutoRestoreEnabled() {
		autoRestore(manager, cfg)
	}
	if cfg.AutoSaveEnabled() {
		registerPendingSave(manager)
	}
}

// autoRestore runs the restore and logs the outcome.
func autoRestore(manager *cachepkg.Manager, cfg *cachepkg.Config) {
	result, err := manager.Restore(context.Background())
	switch {
	case err != nil:
		log.Warn("CI cache: automatic restore failed", fieldKey, cfg.Key, "error", err)
	case result.Hit:
		log.Debug("CI cache: automatic restore hit", fieldKey, cfg.Key, "matched", result.MatchedKey, "exact", result.Exact)
	default:
		log.Debug("CI cache: automatic restore miss", fieldKey, cfg.Key)
	}
}

// registerPendingSave stores the manager for save-on-exit.
func registerPendingSave(manager *cachepkg.Manager) {
	pendingSaveMu.Lock()
	pendingSave = manager
	pendingSaveMu.Unlock()
}

// RunPendingSave performs the registered automatic save-on-exit, if any. It is
// invoked from the CLI Cleanup path so it runs on normal exit and on signal
// termination.
func RunPendingSave() {
	defer perf.Track(nil, "cache.RunPendingSave")()

	pendingSaveMu.Lock()
	manager := pendingSave
	pendingSave = nil
	pendingSaveMu.Unlock()

	if manager == nil {
		return
	}

	result, err := manager.Save(context.Background())
	switch {
	case err != nil:
		log.Warn("CI cache: automatic save failed", "key", manager.Config().Key, "error", err)
	case result.Skipped:
		log.Debug("CI cache: automatic save skipped", "key", manager.Config().Key, "reason", result.Reason)
	default:
		log.Debug("CI cache: automatic save complete", "key", manager.Config().Key)
	}
}

// isCICacheCommand reports whether cmd is one of the `atmos ci cache` subcommands
// (or the `cache` group itself), so the automatic hooks can skip them.
func isCICacheCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "cache" {
			if parent := c.Parent(); parent != nil && parent.Name() == "ci" {
				return true
			}
		}
	}
	return false
}
