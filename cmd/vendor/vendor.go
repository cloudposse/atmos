package vendor

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// vendorCmd executes 'atmos vendor' CLI commands.
var vendorCmd = &cobra.Command{
	Use:                "vendor",
	Short:              "Manage external dependencies for components or stacks",
	Long:               `This command manages external dependencies for Atmos components or stacks by vendoring them. Vendoring involves copying and locking required dependencies locally, ensuring consistency, reliability, and alignment with the principles of immutable infrastructure.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

// vendorPullCmd executes 'vendor pull' CLI commands.
var vendorPullCmd = &cobra.Command{
	Use:                "pull",
	Short:              "Pull the latest vendor configurations or dependencies",
	Long:               "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteVendorPullCmd(cmd, args)
		return err
	},
}

// vendorDiffCmd executes 'vendor diff' CLI commands.
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Show differences in vendor configurations or dependencies",
	Long:               "This command compares and displays the differences in vendor-specific configurations or dependencies.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteVendorDiffCmd(cmd, args)
		return err
	},
}

func init() {
	// Set up vendor pull flags.
	vendorPullCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component")
	vendorPullCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack")
	vendorPullCmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	vendorPullCmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	vendorPullCmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	vendorPullCmd.PersistentFlags().Bool("everything", false, "Vendor all components")

	// Set up vendor diff flags.
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "Compare the differences between the local and vendored versions of the specified component.")
	vendorDiffCmd.PersistentFlags().StringP("type", "t", "terraform", "Compare the differences between the local and vendored versions of the specified component, filtering by type (terraform or helmfile).")
	vendorDiffCmd.PersistentFlags().Bool("dry-run", false, "Simulate the comparison of differences between the local and vendored versions of the specified component without making any changes.")

	// Add subcommands.
	vendorCmd.AddCommand(vendorPullCmd)
	// vendorDiffCmd is not implemented yet, so exclude it from help.
	// vendorCmd.AddCommand(vendorDiffCmd)

	// Register with command registry.
	internal.Register(&VendorCommandProvider{})
}

// VendorCommandProvider implements the CommandProvider interface.
type VendorCommandProvider struct{}

// GetCommand returns the vendor command.
func (v *VendorCommandProvider) GetCommand() *cobra.Command {
	return vendorCmd
}

// GetName returns the command name.
func (v *VendorCommandProvider) GetName() string {
	return "vendor"
}

// GetGroup returns the command group.
func (v *VendorCommandProvider) GetGroup() string {
	return "Component Lifecycle"
}

// GetFlagsBuilder returns the flags builder for this command.
func (v *VendorCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (v *VendorCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (v *VendorCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for vendor command).
func (v *VendorCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (v *VendorCommandProvider) IsExperimental() bool {
	return false
}
