package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGeneratePlanfileCmd generates planfile for a terraform component.
var terraformGeneratePlanfileCmd = &cobra.Command{
	Use:                "planfile",
	Short:              "Generate a planfile for a Terraform component",
	Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
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

		err := e.ExecuteTerraformGeneratePlanfileCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGeneratePlanfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGeneratePlanfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	terraformGeneratePlanfileCmd.PersistentFlags().String("format", "json", "Output format (`json` or `yaml`, `json` is default)")

	if err := terraformGeneratePlanfileCmd.MarkPersistentFlagRequired("stack"); err != nil {
		panic(err)
	}

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
