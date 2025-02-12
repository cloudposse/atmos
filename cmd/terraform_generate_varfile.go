package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateVarfileCmd generates varfile for a terraform component
var terraformGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Generate a varfile for a Terraform component",
	Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfileCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	terraformGenerateVarfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate varfile <component> -s <stack>")
	AddStackCompletion(terraformGenerateVarfileCmd)
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "atmos terraform generate varfile <component> -s <stack> -f <file>")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
