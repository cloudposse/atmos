package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured tools and their installation status",
	Long:  `List all tools configured in .tool-versions file, showing their installation status, install date, and file size.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.RunList()
	},
}
