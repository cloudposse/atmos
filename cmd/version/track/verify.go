package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackVerifyCmd = &cobra.Command{
	Use:   "verify [track]",
	Short: "Verify that a version track is locked and current",
	Long:  "Fail when any configured entry is unlocked or has a policy-eligible update available. Intended for CI.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.verify.RunE")()

		status, err := manager.VerifyTrack(atmosConfig, trackFromArgs(cmd, args))
		if err != nil {
			return err
		}
		return writeFormatted(cmd, status)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions()...).RegisterFlags(trackVerifyCmd)
	trackCmd.AddCommand(trackVerifyCmd)
}
