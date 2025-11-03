package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

var helmfileGenerateVarfileParser = flags.NewStandardOptionsBuilder().
	WithStack(true). // Required.
	WithFile().
	Build()

// helmfileGenerateVarfileCmd generates varfile for a helmfile component.
var helmfileGenerateVarfileCmd = &cobra.Command{
	Use:               "varfile",
	Short:             "Generate a values file for a Helmfile component",
	Long:              "This command generates a values file for a specified Helmfile component.",
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags using StandardOptions.
		_, err := helmfileGenerateVarfileParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Validate component argument.
		if len(args) != 1 {
			return errUtils.ErrInvalidArgumentError
		}

		// Call original implementation with cmd (it needs cmd for other flags like process-templates).
		err = e.ExecuteHelmfileGenerateVarfileCmd(cmd, args)
		return err
	},
}

func init() {
	helmfileGenerateVarfileCmd.DisableFlagParsing = true // IMPORTANT: Manual parsing required for our unified parser

	// Register StandardOptions flags.
	helmfileGenerateVarfileParser.RegisterFlags(helmfileGenerateVarfileCmd)
	_ = helmfileGenerateVarfileParser.BindToViper(viper.GetViper())

	// Add stack completion.
	AddStackCompletion(helmfileGenerateVarfileCmd)

	helmfileGenerateCmd.AddCommand(helmfileGenerateVarfileCmd)
}
