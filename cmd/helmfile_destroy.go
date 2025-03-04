package cmd

import "github.com/spf13/cobra"

// Command: atmos helmfile destroy
var (
	helmfileDestroyShort = "Destroy the Helm releases for the specified stack."
	helmfileDestroyLong  = `This command removes the specified Helm releases from the cluster, ensuring a clean state for
the given stack.

Example usage:
  atmos helmfile destroy echo-server --stack=tenant1-ue2-dev
  atmos helmfile destroy echo-server --stack=tenant1-ue2-dev --redirect-stderr /dev/stdout`
)

// helmfileDestroyCmd represents the base command for all helmfile sub-commands
var helmfileDestroyCmd = &cobra.Command{
	Use:                "destroy",
	Aliases:            []string{},
	Short:              helmfileDestroyShort,
	Long:               helmfileDestroyLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		helmfileRun(cmd, "destroy", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileDestroyCmd)
}
