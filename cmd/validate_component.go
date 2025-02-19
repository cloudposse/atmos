package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:   "component",
	Short: "Validate an Atmos component in a stack using JSON Schema or OPA policies",
	Long:  "This command validates an Atmos component within a stack using JSON Schema or OPA policies.",
	Example: "$ atmos validate component &ltcomponent&gt -s &ltstack&gt\n\n" +
		"$ atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type &ltjsonschema|opa&gt\n\n" +
		"$ atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type opa --module-paths catalog",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		component, stack, err := e.ExecuteValidateComponentCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}

		m := fmt.Sprintf("component `%s` in stack `%s` validated successfully\n", component, stack)
		u.PrintMessageInColor(m, theme.Colors.Success)
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type &ltjsonschema|opa&gt")
	AddStackCompletion(validateComponentCmd)
	validateComponentCmd.PersistentFlags().String("schema-path", "", "atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type &ltjsonschema|opa&gt")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type &ltjsonschema|opa&gt")
	validateComponentCmd.PersistentFlags().StringSlice("module-paths", nil, "atmos validate component &ltcomponent&gt -s &ltstack&gt --schema-path &ltschema_path&gt --schema-type opa --module-paths catalog")
	validateComponentCmd.PersistentFlags().Int("timeout", 0, "Validation timeout in seconds: atmos validate component &ltcomponent&gt -s &ltstack&gt --timeout 15")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	validateCmd.AddCommand(validateComponentCmd)
}
