package cmd

import (
	"github.com/spf13/cobra"
)

// Command: atmos helmfile apply
var (
	helmfileApplyShort = "Apply changes to align the actual state of Helm releases with the desired state."
	helmfileApplyLong  = `This command reconciles the actual state of Helm releases in the cluster with the desired state
defined in your configurations by applying the necessary changes.

Example usage:
  atmos helmfile apply echo-server -s tenant1-ue2-dev
  atmos helmfile apply echo-server -s tenant1-ue2-dev --redirect-stderr /dev/stdout
`
)

// helmfileApplyCmd represents the base command for all helmfile sub-commands
var helmfileApplyCmd = &cobra.Command{
	Use:                "apply",
	Aliases:            []string{},
	Short:              helmfileApplyShort,
	Long:               helmfileApplyLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "apply", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileApplyCmd)
}
