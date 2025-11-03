package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// helmfileGenerateVarfileCmd generates varfile for a helmfile component
var helmfileGenerateVarfileCmd = &cobra.Command{
	Use:               "varfile",
	Short:             "Generate a values file for a Helmfile component",
	Long:              "This command generates a values file for a specified Helmfile component.",
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteHelmfileGenerateVarfileCmd(cmd, args)
		return err
	},
}

func init() {
	helmfileGenerateVarfileCmd.DisableFlagParsing = false
	AddStackCompletion(helmfileGenerateVarfileCmd)
	helmfileGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "Generate a variables file for the specified Helmfile component in the given stack and write the output to the provided file path.")

	err := helmfileGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	helmfileGenerateCmd.AddCommand(helmfileGenerateVarfileCmd)
}
