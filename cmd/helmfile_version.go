package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileVersionCmd returns the Helmfile version.
var helmfileVersionCmd = &cobra.Command{
	Use:                "version",
	Short:              "Get Helmfile version",
	Long:               "This command returns Helmfile version.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		helmfileRun(cmd, "version", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileVersionCmd)
}
