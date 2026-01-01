package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [tool@version]",
	Short: "Uninstall a tool or all tools from .tool-versions",
	Long:  `Uninstall a specific tool version or all tools listed in .tool-versions.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolSpec := ""
		if len(args) > 0 {
			toolSpec = args[0]
		}
		return toolchain.RunUninstall(toolSpec)
	},
}

// UninstallCommandProvider implements the CommandProvider interface.
type UninstallCommandProvider struct{}

func (u *UninstallCommandProvider) GetCommand() *cobra.Command {
	return uninstallCmd
}

func (u *UninstallCommandProvider) GetName() string {
	return "uninstall"
}

func (u *UninstallCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (u *UninstallCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (u *UninstallCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (u *UninstallCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
