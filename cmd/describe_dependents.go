package cmd

import (
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
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeDependentsCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	describeDependentsCmd.DisableFlagParsing = false

	describeDependentsCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe dependents &ltcomponent&gt -s &ltstack&gt")
	AddStackCompletion(describeDependentsCmd)
	describeDependentsCmd.PersistentFlags().StringP("format", "f", "json", "The output format: atmos describe dependents &ltcomponent&gt -s &ltstack&gt --format=json|yaml (`json` is default)")
	describeDependentsCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe dependents &ltcomponent&gt -s &ltstack&gt --file dependents.yaml")

	err := describeDependentsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	describeCmd.AddCommand(describeDependentsCmd)
}
