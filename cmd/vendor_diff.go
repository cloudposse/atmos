package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

var vendorDiffParser = flags.NewStandardOptionsBuilder().
	WithComponent(false).
	WithType("terraform").
	WithDryRun().
	Build()

// vendorDiffCmd executes 'vendor diff' CLI commands.
var vendorDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences in vendor configurations or dependencies",
	Long:  "This command compares and displays the differences in vendor-specific configurations or dependencies.",
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// TODO: There was no documentation here:https://atmos.tools/cli/commands/vendor we need to know what this command requires to check if we should add usage help

		// Check Atmos configuration
		checkAtmosConfig()

		opts, err := vendorDiffParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		err = e.ExecuteVendorDiffCmd(opts)
		return err
	},
}

func init() {
	vendorDiffParser.RegisterFlags(vendorDiffCmd)
	AddStackCompletion(vendorDiffCmd)
	_ = vendorDiffParser.BindToViper(viper.GetViper())

	// Since this command is not implemented yet, exclude it from `atmos help`
	// vendorCmd.AddCommand(vendorDiffCmd)
}
