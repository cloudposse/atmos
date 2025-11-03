package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGeneratePlanfileCmd generates planfile for a terraform component.
var terraformGeneratePlanfileCmd = &cobra.Command{
	Use:                "planfile",
	Short:              "Generate a planfile for a Terraform component",
	Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGeneratePlanfileCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGeneratePlanfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGeneratePlanfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	terraformGeneratePlanfileCmd.PersistentFlags().String("format", "json", "Output format (`json` or `yaml`, `json` is default)")

	err := terraformGeneratePlanfileCmd.MarkPersistentFlagRequired("stack")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
