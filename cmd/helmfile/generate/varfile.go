package generate

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// varfileCmd generates varfile for a helmfile component.
var varfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Generate a values file for a Helmfile component",
	Long:               "This command generates a values file for a specified Helmfile component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               e.ExecuteHelmfileGenerateVarfileCmd,
}

func init() {
	varfileCmd.DisableFlagParsing = false
	varfileCmd.PersistentFlags().StringP("stack", "s", "", "Specify the stack name")
	varfileCmd.PersistentFlags().StringP("file", "f", "", "Generate a variables file for the specified Helmfile component in the given stack and write the output to the provided file path.")

	err := varfileCmd.MarkPersistentFlagRequired("stack")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	GenerateCmd.AddCommand(varfileCmd)
}
