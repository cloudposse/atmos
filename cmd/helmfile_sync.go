package cmd

import "github.com/spf13/cobra"

// Command: atmos helmfile sync
var (
	helmfileSyncShort = "Synchronize the state of Helm releases by reconciling the actual state with the desired state (applies changes as needed)."
	helmfileSyncLong  = `This command ensures that the actual state of Helm releases in the cluster matches the desired
state defined in your configurations by applying any necessary changes.

Example usage:
  atmos helmfile sync echo-server --stack tenant1-ue2-dev
  atmos helmfile sync echo-server --stack tenant1-ue2-dev --redirect-stderr ./errors.txt`
)

// helmfileSyncCmd represents the base command for all helmfile sub-commands
var helmfileSyncCmd = &cobra.Command{
	Use:                "sync",
	Aliases:            []string{},
	Short:              helmfileSyncShort,
	Long:               helmfileSyncLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "sync", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileSyncCmd)
}
