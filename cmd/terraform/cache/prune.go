package cache

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
)

const defaultPruneOlderThan = 168 * time.Hour // Matches the default stale-while-revalidate window.

var pruneParser *flags.StandardParser

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale cached metadata",
	Long: `Remove cached registry metadata older than the retention window. Immutable
artifacts (provider zips, module archives) are kept unless --all is given.`,
	Example: `  atmos terraform cache prune
  atmos terraform cache prune --older-than 720h
  atmos terraform cache prune --all --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defer perf.Track(atmosConfigPtr, "cache.prune.RunE")()

		v := viper.GetViper()
		if err := pruneParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		olderThan, err := time.ParseDuration(v.GetString("older-than"))
		if err != nil {
			return fmt.Errorf("%w: invalid --older-than value %q: %w", errUtils.ErrInvalidConfig, v.GetString("older-than"), err)
		}
		includeArtifacts := v.GetBool("all")
		dryRun := v.GetBool("dry-run")

		root, err := resolveCacheRoot()
		if err != nil {
			return err
		}

		pruned, err := tfcache.Prune(root, olderThan, includeArtifacts, dryRun)
		if err != nil {
			return err
		}

		var freed int64
		for i := range pruned {
			freed += pruned[i].Size
		}
		verb := "Pruned"
		if dryRun {
			verb = "Would prune"
		}
		ui.Info(fmt.Sprintf("%s %d object(s), %s", verb, len(pruned), humanize.Bytes(uint64(freed))))
		return nil
	},
}

func init() {
	pruneParser = flags.NewStandardParser(
		flags.WithStringFlag("older-than", "", defaultPruneOlderThan.String(), "Prune objects older than this duration (e.g. 168h, 720h)"),
		flags.WithBoolFlag("all", "", false, "Also prune immutable artifacts (provider zips, module archives)"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be pruned without deleting"),
		flags.WithEnvVars("older-than", "ATMOS_TERRAFORM_CACHE_PRUNE_OLDER_THAN"),
	)
	pruneParser.RegisterFlags(pruneCmd)
	if err := pruneParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
