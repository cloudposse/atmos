package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var (
	showAllVersions bool
	versionLimit    int
)

var getCmd = &cobra.Command{
	Use:   "get [tool]",
	Short: "Get version information for a tool",
	Long:  `Display version information for a tool from .tool-versions file.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := ""
		if len(args) > 0 {
			toolName = args[0]
		}
		return toolchain.ListToolVersions(showAllVersions, versionLimit, toolName)
	},
}

