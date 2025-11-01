package cmd

import "github.com/spf13/cobra"

// Command: atmos helmfile sync
var (
	helmfileSyncShort = "Synchronize the state of Helm releases with the desired state without making changes."
	helmfileSyncLong  = `This command ensures that the actual state of Helm releases in the cluster matches the desired
state defined in your configurations without performing destructive actions.

Example usage:
  atmos helmfile sync echo-server --stack tenant1-ue2-dev
  atmos helmfile sync echo-server --stack tenant1-ue2-dev --redirect-stderr ./errors.txt`
)

// helmfileSyncCmd represents the base command for all helmfile sub-commands
var helmfileSyncCmd = &cobra.Command{
	Use:     "sync",
	Aliases: []string{},
	Short:   helmfileSyncShort,
	Long:    helmfileSyncLong, RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "sync", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	helmfileParser.RegisterFlags(helmfileSyncCmd)
	helmfileCmd.AddCommand(helmfileSyncCmd)
}
