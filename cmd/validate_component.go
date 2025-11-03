package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
)

var validateComponentParser = flags.NewStandardOptionsBuilder().
	WithStack(true).
	WithSchemaPath("").
	WithSchemaType("").
	WithModulePaths().
	WithTimeout(0).
	Build()

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:               "component",
	Short:             "Validate an Atmos component in a stack using JSON Schema or OPA policies",
	Long:              "This command validates an Atmos component within a stack using JSON Schema or OPA policies.",
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		opts, err := validateComponentParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		component, stack, err := e.ExecuteValidateComponentCmd(opts)
		if err != nil {
			return err
		}

		log.Info("Validated successfully", "component", component, "stack", stack)
		return nil
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentParser.RegisterFlags(validateComponentCmd)
	AddStackCompletion(validateComponentCmd)
	_ = validateComponentParser.BindToViper(viper.GetViper())

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	validateCmd.AddCommand(validateComponentCmd)
}
