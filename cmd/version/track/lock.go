package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackLockCmd = &cobra.Command{
	Use:   "lock [track]",
	Short: "Resolve desired versions and write the lock file",
	Long:  "Resolve each entry's desired version to a concrete version and write versions.lock.yaml. Lock resolves the desired versions as-is; use `update` to advance versions within the update policy.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.lock.RunE")()

		group, _ := cmd.Flags().GetString("group")
		lock, err := manager.LockTrack(atmosConfig, trackFromArgs(cmd, args), group)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, lock)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions(groupFlagOption())...).RegisterFlags(trackLockCmd)
	trackCmd.AddCommand(trackLockCmd)
}
