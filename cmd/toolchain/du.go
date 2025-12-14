package toolchain

import (
	"github.com/spf13/cobra"

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

// DuCommandProvider implements the CommandProvider interface.
type DuCommandProvider struct{}

func (d *DuCommandProvider) GetCommand() *cobra.Command {
	return duCmd
}

func (d *DuCommandProvider) GetName() string {
	return "du"
}

func (d *DuCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (d *DuCommandProvider) GetFlagsBuilder() interface{} {
	return nil // No flags for du command.
}

// Provider instance for automatic registration.
var _ interface{} = (*DuCommandProvider)(nil)
