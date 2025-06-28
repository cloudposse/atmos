package cmd

import (
	"github.com/spf13/cobra"

	atmoserr "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGeneratePlanfileCmd generates planfile for a terraform component.
var terraformGeneratePlanfileCmd = &cobra.Command{
	Use:                "planfile",
	Short:              "Generate a planfile for a Terraform component",
	Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGeneratePlanfileCmd(cmd, args)
		atmoserr.CheckErrorPrintAndExit(err, "", "")
	},
}

func init() {
	terraformGeneratePlanfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGeneratePlanfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	terraformGeneratePlanfileCmd.PersistentFlags().String("format", "json", "Output format (`json` or `yaml`, `json` is default)")

	err := terraformGeneratePlanfileCmd.MarkPersistentFlagRequired("stack")
	atmoserr.CheckErrorPrintAndExit(err, "", "")

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
