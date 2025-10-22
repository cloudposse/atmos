package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorUpdateCmd executes 'vendor update' CLI commands.
var vendorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update version references in vendor configurations to their latest versions",
	Long: `This command checks upstream Git sources for newer versions and updates the version references in vendor configuration files.

The command supports checking Git repositories for newer tags and commits, and will preserve YAML structure including anchors, comments, and formatting.

Use the --check flag to see what updates are available without making changes.
Use the --pull flag to both update version references and pull the new components.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Vendor update doesn't require stack validation
		checkAtmosConfig()

		err := e.ExecuteVendorUpdateCmd(cmd, args)
		return err
	},
}

func init() {
	vendorUpdateCmd.PersistentFlags().Bool("check", false, "Check for updates without modifying configuration files (dry-run mode)")
	vendorUpdateCmd.PersistentFlags().Bool("pull", false, "Update version references AND pull the new component versions")
	vendorUpdateCmd.PersistentFlags().StringP("component", "c", "", "Update version for the specified component name")
	_ = vendorUpdateCmd.RegisterFlagCompletionFunc("component", ComponentsArgCompletion)
	vendorUpdateCmd.PersistentFlags().String("tags", "", "Update versions for components with the specified tags (comma-separated)")
	vendorUpdateCmd.PersistentFlags().StringP("type", "t", "terraform", "Component type: terraform or helmfile")
	vendorUpdateCmd.PersistentFlags().Bool("outdated", false, "Show only components with available updates (use with --check)")
	vendorCmd.AddCommand(vendorUpdateCmd)
}
