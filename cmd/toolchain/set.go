package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var setCmd = &cobra.Command{
	Use:   "set <tool> [version]",
	Short: "Set default version for a tool",
	Long:  `Set the default version for a tool in .tool-versions file. If no version is provided, shows an interactive selector.`,
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		version := ""
		if len(args) > 1 {
			version = args[1]
		}
		return toolchain.SetToolVersion(toolName, version, 0)
	},
}

// SetCommandProvider implements the CommandProvider interface.
type SetCommandProvider struct{}

func (s *SetCommandProvider) GetCommand() *cobra.Command {
	return setCmd
}

func (s *SetCommandProvider) GetName() string {
	return "set"
}

func (s *SetCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (s *SetCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (s *SetCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (s *SetCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
