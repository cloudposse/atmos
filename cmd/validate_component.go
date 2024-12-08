package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:   "component",
	Short: "Execute 'validate component' command",
	Long:  `This command validates an atmos component in a stack using Json Schema or OPA policies: atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>`,
	Example: "atmos validate component <component> -s <stack>\n" +
		"atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>\n" +
		"atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type opa --module-paths catalog",
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		componentList, err := l.FilterAndListComponents(toComplete)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		return componentList, cobra.ShellCompDirectiveNoFileComp
	},

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		component, stack, err := e.ExecuteValidateComponentCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		m := fmt.Sprintf("component '%s' in stack '%s' validated successfully\n", component, stack)
		u.PrintMessageInColor(m, color.New(color.FgGreen))
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>")
	validateComponentCmd.PersistentFlags().String("schema-path", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>")
	validateComponentCmd.PersistentFlags().StringSlice("module-paths", nil, "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type opa --module-paths catalog")
	validateComponentCmd.PersistentFlags().Int("timeout", 0, "Validation timeout in seconds: atmos validate component <component> -s <stack> --timeout 15")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	// Autocompletion for stack flag
	validateComponentCmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		stacksList, err := l.FilterAndListStacks(toComplete)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		return stacksList, cobra.ShellCompDirectiveNoFileComp
	},
	)

	validateCmd.AddCommand(validateComponentCmd)
}
