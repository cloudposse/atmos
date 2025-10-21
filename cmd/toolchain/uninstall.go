package toolchain

import (
	"github.com/spf13/cobra"

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

