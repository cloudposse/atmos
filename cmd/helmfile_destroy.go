package cmd

import "github.com/spf13/cobra"

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
