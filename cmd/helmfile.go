package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Execute 'helmfile' commands",
	Long:               `This command runs Helmfile commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile <helmfile_command> <component> -s <stack>")
	addUsageCommand(helmfileCmd, false)
	RootCmd.AddCommand(helmfileCmd)
}
