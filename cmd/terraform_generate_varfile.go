package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateVarfileCmd generates varfile for a terraform component
var terraformGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Execute 'terraform generate varfile' command",
	Long:               `This command generates a varfile for an atmos terraform component: atmos terraform generate varfile <component> -s <stack> -f <file>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfileCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	terraformGenerateVarfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate varfile <component> -s <stack>")
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "atmos terraform generate varfile <component> -s <stack> -f <file>")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
