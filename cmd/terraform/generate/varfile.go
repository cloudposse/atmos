package generate

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// varfileCmd generates varfile for a terraform component.
var varfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Generate a varfile for a Terraform component",
	Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteTerraformGenerateVarfileCmd(cmd, args)
		return err
	},
}

func init() {
	varfileCmd.DisableFlagParsing = false

	// Add stack flag (required).
	varfileCmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack (required)")
	if err := varfileCmd.MarkPersistentFlagRequired("stack"); err != nil {
		ui.Error(err.Error())
	}

	varfileCmd.PersistentFlags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack.")

	GenerateCmd.AddCommand(varfileCmd)
}
