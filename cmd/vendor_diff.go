package cmd

import (
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorDiffCmd executes 'vendor diff' CLI commands.
// Deprecated: Use 'atmos vendor update --check' instead.
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Show differences in vendor configurations or dependencies (DEPRECATED: use 'vendor update --check')",
	Long:               "This command compares and displays the differences in vendor-specific configurations or dependencies.\n\nDEPRECATED: This command is deprecated. Please use 'atmos vendor update --check' instead.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Hidden:             true, // Hide from help output
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)

		// Print deprecation notice
		log.Warn("'atmos vendor diff' is deprecated. Please use 'atmos vendor update --check' instead.")

		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteVendorDiffCmd(cmd, args)
		return err
	},
}

func init() {
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "Check for updates for the specified component name from the vendor configuration.")
	vendorDiffCmd.PersistentFlags().String("tags", "", "Check for updates for components with the specified tags (comma-separated).")
	vendorDiffCmd.PersistentFlags().Bool("everything", false, "Check for updates for all configured vendor components.")
	vendorDiffCmd.PersistentFlags().Bool("update", false, "Update the vendor configuration file with the latest versions found.")
	vendorDiffCmd.PersistentFlags().Bool("outdated", false, "Only show components that have updates available.")
	AddStackCompletion(vendorDiffCmd)
	vendorDiffCmd.PersistentFlags().StringP("type", "t", "terraform", "Filter components by type (terraform or helmfile).")

	// Register the vendor diff command
	vendorCmd.AddCommand(vendorDiffCmd)
}
