package vendor

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// vendorCmd is the parent command for all vendor subcommands.
var vendorCmd = &cobra.Command{
	Use:                "vendor",
	Short:              "Manage vendored components and dependencies",
	Long:               `Pull, diff, and update vendored components from remote sources.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	// Attach all subcommands.
	vendorCmd.AddCommand(pullCmd)
	vendorCmd.AddCommand(diffCmd)
	vendorCmd.AddCommand(updateCmd)

	// Register with registry.
	internal.Register(&VendorCommandProvider{})
}

// VendorCommandProvider implements the CommandProvider interface.
type VendorCommandProvider struct{}

// GetCommand returns the vendor command with all subcommands attached.
func (v *VendorCommandProvider) GetCommand() *cobra.Command {
	return vendorCmd
}

// GetName returns the command name.
func (v *VendorCommandProvider) GetName() string {
	return "vendor"
}

// GetGroup returns the command group for help organization.
func (v *VendorCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
// Vendor parent command has no flags.
func (v *VendorCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// Vendor parent command has no positional arguments.
func (v *VendorCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// Vendor command has no compatibility flags.
func (v *VendorCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// Vendor command has no aliases.
func (v *VendorCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}
