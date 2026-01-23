package vendor

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the vendor command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// vendorCmd represents the vendor command.
var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "Manage external dependencies for components or stacks",
	Long: `The vendor command downloads remote artifacts (such as Terraform modules, Helm charts, or other configurations) and stores them locally in your project.

Use this command to fetch and manage external dependencies for your infrastructure components.`,
	Args: cobra.NoArgs,
}

func init() {
	// Add subcommands.
	vendorCmd.AddCommand(pullCmd)
	vendorCmd.AddCommand(diffCmd)

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
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

// GetGroup returns the command group for help organization.
func (v *VendorCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
// Parent vendor command has no flags itself.
func (v *VendorCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// Parent vendor command has no positional arguments.
func (v *VendorCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// Parent vendor command has no compatibility flags.
func (v *VendorCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// Parent vendor command has no aliases.
func (v *VendorCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (v *VendorCommandProvider) IsExperimental() bool {
	return false
}
