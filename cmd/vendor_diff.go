package cmd

import (
	atmoserr "github.com/cloudposse/atmos/errors"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorDiffCmd executes 'vendor diff' CLI commands
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Show differences in vendor configurations or dependencies",
	Long:               "This command compares and displays the differences in vendor-specific configurations or dependencies.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		// TODO: There was no documentation here:https://atmos.tools/cli/commands/vendor we need to know what this command requires to check if we should add usage help

		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteVendorDiffCmd(cmd, args)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
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
