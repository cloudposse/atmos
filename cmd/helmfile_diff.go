package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileDiffCmd represents the base command for all helmfile sub-commands
var helmfileDiffCmd = &cobra.Command{
	Use:                "diff",
	Aliases:            []string{},
	Short:              helmfileDiffShort,
	Long:               helmfileDiffLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		helmfileRun(cmd, "diff", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileDiffCmd)
}
