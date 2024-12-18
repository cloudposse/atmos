package cmd

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "List Atmos components that depend on a given component",
	Long:               "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeDependentsCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	describeDependentsCmd.DisableFlagParsing = false

	describeDependentsCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe dependents <component> -s <stack>")
	describeDependentsCmd.PersistentFlags().StringP("format", "f", "json", "The output format: atmos describe dependents <component> -s <stack> --format=json|yaml ('json' is default)")
	describeDependentsCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe dependents <component> -s <stack> --file dependents.yaml")

	err := describeDependentsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	describeCmd.AddCommand(describeDependentsCmd)
}
