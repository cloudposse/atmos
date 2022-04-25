package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"os"
)

// terraformGenerateVarfileCmd generates varfile for a terraform component
var terraformGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Execute 'terraform generate varfile' command",
	Long:               `This command generates a varfile for a terraform component: atmos terraform generate varfile <component> -s <stack> -f <file>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateVarfile(cmd, args)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	terraformGenerateVarfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate varfile <component> -s <stack>")
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "atmos terraform generate varfile <component> -s <stack> -f <file>")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
