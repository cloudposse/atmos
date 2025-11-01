package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured tools and their installation status",
	Long:  `List all tools configured in .tool-versions file, showing their installation status, install date, and file size.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.RunList()
	},
}
