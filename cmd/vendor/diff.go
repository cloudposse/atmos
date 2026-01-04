package vendor

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	pkgvendor "github.com/cloudposse/atmos/pkg/vendor"
)

// diffCmd represents the vendor diff command.
var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between local and remote vendor dependencies",
	Long: `The vendor diff command compares your local vendored files against the remote sources to identify any differences.

This command is not yet implemented.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "vendor.diff.RunE")()

		return pkgvendor.Diff()
	},
}
