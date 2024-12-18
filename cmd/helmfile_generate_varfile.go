package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// helmfileGenerateVarfileCmd generates varfile for a helmfile component
var helmfileGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Generate a varfile for an Atmos Helmfile component",
	Long:               "This command generates a varfile for a specified Atmos Helmfile component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteHelmfileGenerateVarfileCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	helmfileGenerateVarfileCmd.DisableFlagParsing = false
	helmfileGenerateVarfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile generate varfile <component> -s <stack>")
	helmfileGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "atmos helmfile generate varfile <component> -s <stack> -f <file>")

	err := helmfileGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	helmfileGenerateCmd.AddCommand(helmfileGenerateVarfileCmd)
}
