package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Short:              "Execute 'terraform' commands",
	Long:               `This command runs terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformCmd(cmd, args)
		if err != nil {
			u.LogErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	RootCmd.AddCommand(terraformCmd)
}
