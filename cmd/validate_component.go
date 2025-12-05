package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// validateComponentCmd validates atmos components.
var validateComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Validate an Atmos component in a stack using JSON Schema or OPA policies",
	Long:               "This command validates an Atmos component within a stack using JSON Schema or OPA policies.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		_, _, err := e.ExecuteValidateComponentCmd(cmd, args)
		if err != nil {
			return err
		}

		return nil
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
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	validateCmd.AddCommand(validateComponentCmd)
}
