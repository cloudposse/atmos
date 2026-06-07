package cache

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a cached object by key",
	Long: `Delete a single cached object (and its metadata sidecar) by its cache key, as
shown by 'atmos terraform cache list'.`,
	Example: `  atmos terraform cache delete providers/registry.terraform.io/hashicorp/aws/index.json`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "cache.delete.RunE")()

		key := args[0]
		root, err := resolveCacheRoot()
		if err != nil {
			return err
		}
		if err := tfcache.Delete(root, key); err != nil {
			return err
		}
		ui.Success(fmt.Sprintf("Deleted %s", key))
		return nil
	},
}
