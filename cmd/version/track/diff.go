package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackDiffCmd = &cobra.Command{
	Use:   "diff [track]",
	Short: "Show entries whose locked version differs from the resolved target",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.diff.RunE")()

		group, _ := cmd.Flags().GetString("group")
		status, err := manager.StatusTrack(atmosConfig, trackFromArgs(cmd, args), group)
		if err != nil {
			return err
		}
		var changed []manager.StatusEntry
		for i := range status.Entries {
			if status.Entries[i].Status == manager.StatusUpdateAvailable || status.Entries[i].Status == manager.StatusUnlocked {
				changed = append(changed, status.Entries[i])
			}
		}
		return writeFormatted(cmd, changed)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions(groupFlagOption())...).RegisterFlags(trackDiffCmd)
	trackCmd.AddCommand(trackDiffCmd)
}
