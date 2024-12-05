package cmd

import (
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "Execute 'describe dependents' command",
	Long:               `This command produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component: atmos describe dependents [options]`,
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

	// Autocompletion for stack flag
	describeDependentsCmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		stacksList, err := l.FilterAndListStacks(toComplete)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		return stacksList, cobra.ShellCompDirectiveNoFileComp
	},
	)

	describeCmd.AddCommand(describeDependentsCmd)
}
