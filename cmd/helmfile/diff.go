package helmfile

import "github.com/spf13/cobra"

// Command: atmos helmfile diff.
var (
	helmfileDiffShort = "Show differences between the desired and actual state of Helm releases."
	helmfileDiffLong  = `This command calculates and displays the differences between the desired state of Helm releases
defined in your configurations and the actual state deployed in the cluster.

Example usage:
  atmos helmfile diff echo-server -s tenant1-ue2-dev
  atmos helmfile diff echo-server -s tenant1-ue2-dev --redirect-stderr /dev/null`
)

// helmfileDiffCmd represents the helmfile diff subcommand.
var helmfileDiffCmd = &cobra.Command{
	Use:                "diff",
	Aliases:            []string{},
	Short:              helmfileDiffShort,
	Long:               helmfileDiffLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "diff", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileDiffCmd)
}
