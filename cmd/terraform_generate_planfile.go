package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
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
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	terraformGeneratePlanfileCmd.DisableFlagParsing = false
	terraformGeneratePlanfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	terraformGeneratePlanfileCmd.PersistentFlags().String("format", "json", "Output format (json or yaml)")

	err := terraformGeneratePlanfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
