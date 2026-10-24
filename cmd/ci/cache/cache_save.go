package cache

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/ui"
)

var cacheSaveParser *flags.StandardParser

// cacheSaveCmd archives the cache root and uploads it under the configured key.
var cacheSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save the well-known cache directory to the CI cache",
	Long: `Save the CI cache.

Archives the cache root (toolchain and anything else under it) and uploads it
under the cache key. Cache entries are write-once: when the exact key was an
exact hit at restore time (content unchanged) or has already been saved this
lifecycle, the save is skipped.`,
	Args: cobra.NoArgs,
	RunE: runCacheSave,
}

func runCacheSave(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := cacheSaveParser.BindFlagsToViper(cmd, v); err != nil {
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

	result, err := manager.Save(cmd.Context())
	if err != nil {
		return err
	}

	switch {
	case result.Skipped && result.Saved:
		ui.Info("Cache already present remotely (key: " + cfg.Key + ")")
	case result.Skipped:
		ui.Info("Cache save skipped: " + result.Reason + " (key: " + cfg.Key + ")")
	default:
		ui.Success("Cache saved (key: " + cfg.Key + ")")
	}
	return nil
}

func init() {
	cacheSaveParser = flags.NewStandardParser(
		flags.WithStringFlag("key", "k", "", "Exact cache key (defaults to a key derived from the toolchain lockfile)"),
		flags.WithStringSliceFlag("path", "p", nil, "Root-relative subpaths to save (defaults to the entire cache root)"),
		flags.WithStringFlag("root", "", "", "Override the cache root directory"),
		flags.WithEnvVars("key", "ATMOS_CI_CACHE_KEY"),
		flags.WithEnvVars("path", "ATMOS_CI_CACHE_PATHS"),
		flags.WithEnvVars("root", "ATMOS_CI_CACHE_ROOT"),
	)
	cacheSaveParser.RegisterFlags(cacheSaveCmd)
	if err := cacheSaveParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
