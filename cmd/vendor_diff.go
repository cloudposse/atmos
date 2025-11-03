package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorDiffCmd executes 'vendor diff' CLI commands.
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Show differences in vendor configurations or dependencies",
	Long:               "This command compares and displays the differences in vendor-specific configurations or dependencies.",
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// TODO: There was no documentation here:https://atmos.tools/cli/commands/vendor we need to know what this command requires to check if we should add usage help

		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteVendorDiffCmd(cmd, args)
		return err
	},
}

func init() {
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "Compare the differences between the local and vendored versions of the specified component.")
	AddStackCompletion(vendorDiffCmd)
	vendorDiffCmd.PersistentFlags().StringP("type", "t", "terraform", "Compare the differences between the local and vendored versions of the specified component, filtering by type (terraform or helmfile).")
	vendorDiffCmd.PersistentFlags().Bool("dry-run", false, "Simulate the comparison of differences between the local and vendored versions of the specified component without making any changes.")

	// Since this command is not implemented yet, exclude it from `atmos help`
	// vendorCmd.AddCommand(vendorDiffCmd)
}
