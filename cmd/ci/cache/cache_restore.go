package cache

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/ui"
)

var cacheRestoreParser *flags.StandardParser

// cacheRestoreCmd restores the configured cache into the cache root.
var cacheRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore the cache into the well-known cache directory",
	Long: `Restore the CI cache.

Looks up the exact key first, then falls back to the configured restore-keys
(prefix matches). On a hit, the archive is extracted into the cache root. The
operation is idempotent within a lifecycle: once restored, repeat restores are
no-ops.`,
	Args: cobra.NoArgs,
	RunE: runCacheRestore,
}

func runCacheRestore(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := cacheRestoreParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	manager, cfg, err := cacheSetup(cmd, cacheOverrides{
		key:   v.GetString("key"),
		root:  v.GetString("root"),
		paths: v.GetStringSlice("path"),
	})
	if err != nil {
		return err
	}
	if rk := v.GetStringSlice("restore-key"); len(rk) > 0 {
		cfg.RestoreKeys = rk
	}

	result, err := manager.Restore(cmd.Context())
	if err != nil {
		return err
	}

	switch {
	case result.Skipped:
		ui.Info("Cache already restored this lifecycle (key: " + cfg.Key + ")")
	case result.Hit && result.Exact:
		ui.Success("Cache restored (exact match: " + result.MatchedKey + ")")
	case result.Hit:
		ui.Success("Cache restored (restore-key match: " + result.MatchedKey + ")")
	default:
		ui.Warning("Cache miss (key: " + cfg.Key + ")")
	}
	return nil
}

func init() {
	cacheRestoreParser = flags.NewStandardParser(
		flags.WithStringFlag("key", "k", "", "Exact cache key (defaults to a key derived from the toolchain lockfile)"),
		flags.WithStringSliceFlag("restore-key", "", nil, "Prefix fallback keys tried in order when the exact key is absent"),
		flags.WithStringSliceFlag("path", "p", nil, "Root-relative subpaths to restore (defaults to the entire cache root)"),
		flags.WithStringFlag("root", "", "", "Override the cache root directory"),
		flags.WithEnvVars("key", "ATMOS_CI_CACHE_KEY"),
		flags.WithEnvVars("restore-key", "ATMOS_CI_CACHE_RESTORE_KEYS"),
		flags.WithEnvVars("path", "ATMOS_CI_CACHE_PATHS"),
		flags.WithEnvVars("root", "ATMOS_CI_CACHE_ROOT"),
	)
	cacheRestoreParser.RegisterFlags(cacheRestoreCmd)
	if err := cacheRestoreParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
