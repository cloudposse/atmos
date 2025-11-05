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

var terraformGenerateVarfileParser = flags.NewStandardOptionsBuilder().
	WithStack(true). // Required.
	WithFile().
	Build()

// terraformGenerateVarfileCmd generates varfile for a terraform component.
var terraformGenerateVarfileCmd = &cobra.Command{
	Use:               "varfile",
	Short:             "Generate a varfile for a Terraform component",
	Long:              "This command generates a `varfile` for a specified Atmos Terraform component.",
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags using StandardOptions.
		opts, err := terraformGenerateVarfileParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Validate component argument (use positional args after flag parsing).
		positionalArgs := opts.GetPositionalArgs()
		if len(positionalArgs) != 1 {
			return fmt.Errorf("%w: invalid arguments. The command requires one argument 'component'", errUtils.ErrInvalidArgumentError)
		}

		// Call original implementation with positional args (it needs only the component).
		err = e.ExecuteTerraformGenerateVarfileCmd(cmd, positionalArgs)
		return err
	},
}

func init() {
	// Register StandardOptions flags.
	terraformGenerateVarfileParser.RegisterFlags(terraformGenerateVarfileCmd)
	_ = terraformGenerateVarfileParser.BindToViper(viper.GetViper())

	// Add stack completion.
	AddStackCompletion(terraformGenerateVarfileCmd)

	terraformGenerateCmd.AddCommand(terraformGenerateVarfileCmd)
}
