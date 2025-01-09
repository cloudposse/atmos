package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileApplyCmd represents the base command for all helmfile sub-commands
var helmfileApplyCmd = &cobra.Command{
	Use:                "apply",
	Aliases:            []string{},
	Short:              helmfileApplyShort,
	Long:               helmfileApplyLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		helmfileRun(cmd, "apply", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileApplyCmd)
}
