package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGenerateVarfileCmd generates varfile for a terraform component
var terraformGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Generate a varfile for a Terraform component",
	Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfileCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateVarfileCmd)
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack.")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
