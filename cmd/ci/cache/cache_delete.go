package cache

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/ui"
)

var cacheDeleteParser *flags.StandardParser

// cacheDeleteCmd deletes a cache entry by key.
var cacheDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a CI cache entry by key",
	Long: `Delete a CI cache entry by its exact key.

Deleting a key that does not exist is a no-op.`,
	Args: cobra.NoArgs,
	RunE: runCacheDelete,
}

func runCacheDelete(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := cacheDeleteParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	key := v.GetString("key")
	if key == "" {
		return errUtils.Build(errUtils.ErrCacheKeyRequired).
			WithExplanation("A cache key is required to delete an entry").
			WithHint("Pass the key to delete with --key=<key>").
			Err()
	}

	manager, _, err := cacheAdminSetup(cmd, cacheOverrides{})
	if err != nil {
		return err
	}

	if err := manager.Delete(cmd.Context(), key); err != nil {
		return err
	}

	ui.Success("Cache deleted (key: " + key + ")")
	return nil
}

func init() {
	cacheDeleteParser = flags.NewStandardParser(
		flags.WithStringFlag("key", "k", "", "Exact cache key to delete (required)"),
		flags.WithEnvVars("key", "ATMOS_CI_CACHE_KEY"),
	)
	cacheDeleteParser.RegisterFlags(cacheDeleteCmd)
	if err := cacheDeleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
