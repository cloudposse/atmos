package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var duCmd = &cobra.Command{
	Use:           "du",
	Short:         "Show disk space usage for installed tools",
	Long:          `Display the total disk space consumed by all installed tools in a human-readable format.`,
	Args:          cobra.NoArgs,
	RunE:          runDu,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func runDu(cmd *cobra.Command, args []string) error {
	return toolchain.DuExec()
}

// DuCommandProvider implements the CommandProvider interface for the du command.
// It provides disk space usage information for installed tools.
type DuCommandProvider struct{}

// GetCommand returns the Cobra command for disk usage.
func (d *DuCommandProvider) GetCommand() *cobra.Command {
	return duCmd
}

// GetName returns the command name.
func (d *DuCommandProvider) GetName() string {
	return "du"
}

// GetGroup returns the command group for help display.
func (d *DuCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

// GetFlagsBuilder returns the flags builder (none for du command).
func (d *DuCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder (none for du command).
func (d *DuCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags (none for du command).
func (d *DuCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for du command).
func (d *DuCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// Compile-time check that DuCommandProvider implements CommandProvider.
var _ internal.CommandProvider = (*DuCommandProvider)(nil)
