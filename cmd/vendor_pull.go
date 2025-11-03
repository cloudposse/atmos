package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

var vendorPullParser = flags.NewStandardOptionsBuilder().
	WithComponent(false).
	WithStack(false).
	WithType("terraform").
	WithDryRun().
	WithTags("").
	WithEverything().
	Build()

// vendorPullCmd executes 'vendor pull' CLI commands.
var vendorPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull the latest vendor configurations or dependencies",
	Long:  "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse options to check if stack flag is present
		opts, err := vendorPullParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// WithStackValidation is a functional option that enables/disables stack configuration validation
		// based on whether the --stack flag is provided
		checkAtmosConfig(WithStackValidation(opts.Stack != ""))

		err = e.ExecuteVendorPullCmd(opts)
		return err
	},
}

func init() {
	vendorPullParser.RegisterFlags(vendorPullCmd)
	_ = vendorPullCmd.RegisterFlagCompletionFunc("component", ComponentsArgCompletion)
	AddStackCompletion(vendorPullCmd)
	_ = vendorPullParser.BindToViper(viper.GetViper())

	vendorCmd.AddCommand(vendorPullCmd)
}
