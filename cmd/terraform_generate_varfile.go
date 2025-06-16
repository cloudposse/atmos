package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/telemetry"
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
			telemetry.CaptureCmdFailure(cmd)
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		telemetry.CaptureCmd(cmd)
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateVarfileCmd)
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack.")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		telemetry.CaptureCmdFailure(terraformGenerateVarfileCmd)
		u.LogErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
