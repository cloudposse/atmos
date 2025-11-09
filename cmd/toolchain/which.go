package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var whichCmd = &cobra.Command{
	Use:   "which <tool>",
	Short: "Show path to installed tool binary",
	Long:  `Show the full path to an installed tool binary.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.WhichExec(args[0])
	},
}

// WhichCommandProvider implements the CommandProvider interface.
type WhichCommandProvider struct{}

func (w *WhichCommandProvider) GetCommand() *cobra.Command {
	return whichCmd
}

func (w *WhichCommandProvider) GetName() string {
	return "which"
}

func (w *WhichCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (w *WhichCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (w *WhichCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (w *WhichCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
