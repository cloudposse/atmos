package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// vendorUpdateCmd executes 'vendor update' CLI commands.
var vendorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update version references in vendor configurations to their latest versions",
	Long: `This command checks upstream sources for newer versions and updates the version references in vendor configuration files.

The command supports checking Git repositories for newer tags and commits, and will preserve YAML structure including anchors, comments, and formatting.

Use the --check flag to see what updates are available without making changes.
Use the --pull flag to both update version references and pull the new components.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)

		// Check Atmos configuration
		checkAtmosConfig()

		// Print command info
		u.PrintfMessageToTUI("Executing 'atmos vendor update'\n")

		// Execute the vendor update command
		err := e.ExecuteVendorUpdateCmd(cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
}

//go:embed markdown/atmos_vendor_update_usage.md
var vendorUpdateUsageMarkdown string

func init() {
	// Add flags specific to vendor update
	vendorUpdateCmd.PersistentFlags().Bool("check", false, "Check for updates without modifying configuration files (dry-run mode)")
	vendorUpdateCmd.PersistentFlags().Bool("pull", false, "Update version references AND pull the new component versions")
	vendorUpdateCmd.PersistentFlags().StringP("component", "c", "", "Update version for the specified component name")
	vendorUpdateCmd.PersistentFlags().String("tags", "", "Update versions for components with the specified tags (comma-separated)")
	vendorUpdateCmd.PersistentFlags().StringP("type", "t", "terraform", "Component type: terraform or helmfile")

	// Set the example usage
	vendorUpdateCmd.Example = vendorUpdateUsageMarkdown

	// Register with the vendor command
	vendorCmd.AddCommand(vendorUpdateCmd)
}
