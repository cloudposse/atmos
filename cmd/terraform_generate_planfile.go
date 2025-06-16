package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/telemetry"
	u "github.com/cloudposse/atmos/pkg/utils"
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
		if err != nil {
			telemetry.CaptureCmdFailure(cmd)
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		telemetry.CaptureCmd(cmd)
	},
}

func init() {
	terraformGeneratePlanfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGeneratePlanfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	terraformGeneratePlanfileCmd.PersistentFlags().String("format", "json", "Output format (`json` or `yaml`, `json` is default)")

	err := terraformGeneratePlanfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		telemetry.CaptureCmdFailure(terraformGeneratePlanfileCmd)
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
