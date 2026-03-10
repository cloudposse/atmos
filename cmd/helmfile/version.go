package helmfile

import "github.com/spf13/cobra"

// helmfileVersionCmd returns the Helmfile version.
var helmfileVersionCmd = &cobra.Command{
	Use:                "version",
	Short:              "Get Helmfile version",
	Long:               "This command returns Helmfile version.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "version", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileVersionCmd)
}
