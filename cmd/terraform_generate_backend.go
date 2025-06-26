package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateBackendCmd generates backend config for a terraform component
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		CheckErrorAndExit(err, "", "")
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateBackendCmd)
	err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}
