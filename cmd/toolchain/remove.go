package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var removeCmd = &cobra.Command{
	Use:   "remove <tool[@version]>",
	Short: "Remove a tool or version from .tool-versions file",
	Long:  `Remove a tool or specific version from the .tool-versions file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := toolchain.GetToolVersionsFilePath()
		tool, version, err := toolchain.ParseToolVersionArg(args[0])
		if err != nil {
			return err
		}
		return toolchain.RemoveToolVersion(filePath, tool, version)
	},
}
