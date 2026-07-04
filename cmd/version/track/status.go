package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackStatusCmd = &cobra.Command{
	Use:   "status [track]",
	Short: "Show lock and update status for a version track",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.status.RunE")()

		group, _ := cmd.Flags().GetString("group")
		status, err := manager.StatusTrack(atmosConfig, trackFromArgs(cmd, args), group)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, status)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions(groupFlagOption())...).RegisterFlags(trackStatusCmd)
	trackCmd.AddCommand(trackStatusCmd)
}
