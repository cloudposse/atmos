package vendor

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var vendorPullParser *flags.StandardParser

// componentTypeFlagHelp is the canonical --type help text shared by every vendor subcommand that
// registers the flag (vendor pull, vendor update, vendor diff) -- a single source of truth so a
// future component type (or a corrected description) can't drift out of sync across subcommands
// the way it previously did (this flag said "terraform or helmfile" on vendor pull well after
// packer-inclusive discovery landed on vendor update/diff).
const componentTypeFlagHelp = "Component type (terraform, helmfile, or packer)"

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

func init() {
	// Set up vendor pull flags. Registered via Flags() (not PersistentFlags()): vendorPullCmd has
	// no subcommands of its own, so persistent inheritance was never needed here.
	vendorPullParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Only vendor the specified component"),
		flags.WithStringFlag("type", "t", "terraform", componentTypeFlagHelp),
		flags.WithBoolFlag("dry-run", "", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes."),
		flags.WithStringFlag("tags", "", "", "Only vendor the components that have the specified tags"),
		flags.WithBoolFlag("everything", "", false, "Vendor all components"),
		flags.WithBoolFlag("refresh-lock", "", false, "Refresh immutable vendor lock entries from declared sources"),
		flags.WithStringFlag("lock-enforcement", "", "", "Override vendor.lock.enforcement (strict, warn, or silent)"),
	)
	vendorPullParser.RegisterFlags(vendorPullCmd)
	if err := vendorPullParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add subcommands. The 'clean', 'update', 'diff', 'get', and 'set' subcommands are
	// attached in their own files' init() functions.
	vendorCmd.AddCommand(vendorPullCmd)

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
