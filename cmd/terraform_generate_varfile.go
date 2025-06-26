package cmd

import (
	"github.com/spf13/cobra"

	atmoserr "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
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
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
	},
}

func init() {
	terraformGenerateVarfileCmd.DisableFlagParsing = false
	AddStackCompletion(terraformGenerateVarfileCmd)
	terraformGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack.")

	err := terraformGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
