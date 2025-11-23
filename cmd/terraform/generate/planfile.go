package generate

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// planfileCmd generates planfile for a terraform component.
var planfileCmd = &cobra.Command{
	Use:                "planfile",
	Short:              "Generate a planfile for a Terraform component",
	Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteTerraformGeneratePlanfileCmd(cmd, args)
		return err
	},
}

func init() {
	planfileCmd.DisableFlagParsing = false

	// Add stack flag (required).
	planfileCmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack (required)")
	if err := planfileCmd.MarkPersistentFlagRequired("stack"); err != nil {
		ui.Error(err.Error())
	}

	planfileCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	planfileCmd.PersistentFlags().String("format", "json", "Output format (`json` or `yaml`, `json` is default)")

	GenerateCmd.AddCommand(planfileCmd)
}
