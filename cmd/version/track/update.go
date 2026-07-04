package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackUpdateCmd = &cobra.Command{
	Use:   "update [track]",
	Short: "Update locked versions within the update policy",
	Long:  "Advance locked versions to the newest candidates allowed by each entry's effective update policy (strategy caps, cooldown, allow/ignore rules) and write the lock file. Newer versions held back by policy are reported with the blocking reason. Use `lock` to resolve desired versions as-is.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.update.RunE")()

		group, _ := cmd.Flags().GetString("group")
		only, _ := cmd.Flags().GetStringSlice("only")
		update, err := manager.UpdateTrackWithContext(cmd.Context(), atmosConfig, trackFromArgs(cmd, args), group, only)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, update)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions(
		groupFlagOption(),
		flags.WithStringSliceFlag("only", "", nil, "Limit the update to the named entries (repeatable)"),
	)...).RegisterFlags(trackUpdateCmd)
	trackCmd.AddCommand(trackUpdateCmd)
}
