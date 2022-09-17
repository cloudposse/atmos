package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// helmfileCmd represents the base command for all terraform sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Short:              "Execute 'helmfile' commands",
	Long:               `This command runs helmfile commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteHelmfile(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile <helmfile_command> <component> -s <stack>")
	RootCmd.AddCommand(helmfileCmd)
}
