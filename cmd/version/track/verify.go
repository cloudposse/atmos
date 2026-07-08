package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

var trackVerifyCmd = &cobra.Command{
	Use:   "verify [track]",
	Short: "Verify that a version track is locked, current, and applied",
	Long:  "Fail when any configured entry is unlocked, has a policy-eligible update available, or when version-managed files differ from the lock. Intended for CI.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.verify.RunE")()

		track := trackFromArgs(cmd, args)
		status, err := manager.VerifyTrackWithContext(cmd.Context(), atmosConfig, track)
		if err != nil {
			return err
		}
		planned, err := managers.Plan(cmd.Context(), &managers.RunOptions{
			Config: atmosConfig,
			Track:  track,
			Render: renderTemplate,
		})
		if err != nil {
			return err
		}
		if err := managers.Check(planned); err != nil {
			return err
		}
		return writeFormatted(cmd, status)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions()...).RegisterFlags(trackVerifyCmd)
	trackCmd.AddCommand(trackVerifyCmd)
}
