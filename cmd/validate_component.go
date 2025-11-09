package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateComponentCmd validates atmos components
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

		// Handle path-based component resolution
		if len(args) > 0 {
			component := args[0]
			needsPathResolution := comp.IsExplicitComponentPath(component)

			if needsPathResolution {
				flags := cmd.Flags()
				stack, err := flags.GetString("stack")
				if err != nil {
					return err
				}

				if stack == "" {
					return errors.New("--stack flag is required when using path-based component resolution")
				}

				// Load atmos configuration
				atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
					ComponentFromArg: component,
					Stack:            stack,
				}, false)
				if err != nil {
					return err
				}

				// Extract component info from path without type checking or stack validation.
				// Validate component will detect the component type, and stack validation
				// happens later in ExecuteValidateComponentCmd after stacks are loaded.
				componentInfo, err := u.ExtractComponentInfoFromPath(&atmosConfig, component)
				if err != nil {
					return fmt.Errorf("path resolution failed: %w", err)
				}

				// Replace the argument with the resolved component name
				args[0] = componentInfo.FullComponent
			}
		}

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
