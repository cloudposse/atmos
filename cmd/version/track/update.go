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
	Long:  "Advance locked versions to the newest candidates allowed by each entry's effective update policy (strategy, cooldown, allow/ignore) and write the lock file.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.update.RunE")()

		group, _ := cmd.Flags().GetString("group")
		lock, err := manager.LockTrack(atmosConfig, trackFromArgs(cmd, args), group)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, lock)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions(groupFlagOption())...).RegisterFlags(trackUpdateCmd)
	trackCmd.AddCommand(trackUpdateCmd)
}
