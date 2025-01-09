package cmd

import "github.com/spf13/cobra"

// helmfileSyncCmd represents the base command for all helmfile sub-commands
var helmfileSyncCmd = &cobra.Command{
	Use:                "sync",
	Aliases:            []string{},
	Short:              helmfileSyncShort,
	Long:               helmfileSyncLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		helmfileRun(cmd, "sync", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileSyncCmd)
}
