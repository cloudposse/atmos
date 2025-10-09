package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGenerateBackendCmd generates backend config for a terraform component
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := handleHelpRequest(cmd, args); err != nil {
			return err
		}
		// Check Atmos configuration
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateBackendCmd)
	if err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack"); err != nil {
		panic(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}
