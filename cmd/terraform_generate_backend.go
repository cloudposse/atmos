package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateBackendCmd generates backend config for a terraform component
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	Example:            `atmos terraform generate backend <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	terraformGenerateBackendCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate backend <component> -s <stack>")

	err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}
