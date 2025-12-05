package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGenerateBackendCmd generates backend config for a terraform component.
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateBackendCmd)
	err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}
