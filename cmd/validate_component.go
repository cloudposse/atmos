package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Validate an Atmos component in a stack using JSON Schema or OPA policies",
	Long:               "This command validates an Atmos component within a stack using JSON Schema or OPA policies.",
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

	AddStackCompletion(validateComponentCmd)
	validateComponentCmd.PersistentFlags().String("schema-path", "", "Specify the path to the schema file used for validating the component configuration in the given stack, supporting schema types like jsonschema or opa.")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "Validate the specified component configuration in the given stack using the provided schema file path and schema type (`jsonschema` or `opa`).")
	validateComponentCmd.PersistentFlags().StringSlice("module-paths", nil, "Specify the paths to OPA policy modules or catalogs used for validating the component configuration in the given stack.")
	validateComponentCmd.PersistentFlags().Int("timeout", 0, "Validation timeout in seconds")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}
	validateComponentCommandConfig()
	validateCmd.AddCommand(validateComponentCmd)
}

func validateComponentCommandConfig() {
	config.DefaultConfigHandler.AddConfig(validateComponentCmd, config.ConfigOptions{
		FlagName:     "schemas-jsonschema-dir",
		EnvVar:       "ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH",
		Description:  "Base path for JSON Schema files.",
		Key:          "schemas.jsonschema.base_path",
		DefaultValue: "",
	})
	config.DefaultConfigHandler.AddConfig(validateComponentCmd, config.ConfigOptions{
		FlagName:     "schemas-opa-dir",
		EnvVar:       "ATMOS_SCHEMAS_OPA_BASE_PATH",
		Description:  "Base path for OPA policy files.",
		Key:          "schemas.opa.base_path",
		DefaultValue: "",
	})

}
