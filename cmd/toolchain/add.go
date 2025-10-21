package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var addCmd = &cobra.Command{
	Use:   "add <tool@version>",
	Short: "Add a tool to .tool-versions file",
	Long:  `Add a tool and version to the .tool-versions file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool, version, err := toolchain.ParseToolVersionArg(args[0])
		if err != nil {
			return err
		}
		return toolchain.AddToolVersion(tool, version)
	},
}

