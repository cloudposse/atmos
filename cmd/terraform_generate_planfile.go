package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

var terraformGeneratePlanfileParser = flags.NewStandardOptionsBuilder().
	WithStack(true). // Required.
	WithFile().
	WithFormat([]string{"json", "yaml"}, "json").
	WithProcessTemplates(true).
	WithProcessFunctions(true).
	Build()

// terraformGeneratePlanfileCmd generates planfile for a terraform component.
var terraformGeneratePlanfileCmd = &cobra.Command{
	Use:               "planfile",
	Short:             "Generate a planfile for a Terraform component",
	Long:              "This command generates a `planfile` for a specified Atmos Terraform component.",
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags using StandardOptions.
		opts, err := terraformGeneratePlanfileParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Validate component argument (use positional args after flag parsing).
		positionalArgs := opts.GetPositionalArgs()
		if len(positionalArgs) != 1 {
			return fmt.Errorf("%w: invalid arguments. The command requires one argument 'component'", errUtils.ErrInvalidArgumentError)
		}

		// Call original implementation with positional args (it needs only the component).
		err = e.ExecuteTerraformGeneratePlanfileCmd(cmd, positionalArgs)
		return err
	},
}

func init() {
	// Register StandardOptions flags.
	terraformGeneratePlanfileParser.RegisterFlags(terraformGeneratePlanfileCmd)
	_ = terraformGeneratePlanfileParser.BindToViper(viper.GetViper())

	// Add stack completion.
	AddStackCompletion(terraformGeneratePlanfileCmd)

	terraformGenerateCmd.AddCommand(terraformGeneratePlanfileCmd)
}
