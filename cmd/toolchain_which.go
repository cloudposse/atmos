package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainWhichCmd = &cobra.Command{
	Use:   "which <tool>",
	Short: "Display the path to an executable",
	Long: `Display the path to an executable for a given tool.

This command shows the full path to the binary for a tool that is configured
in .tool-versions and installed via toolchain.

Examples:
  toolchain which terraform
  toolchain which hashicorp/terraform
  toolchain which kubectl`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		return toolchain.WhichExec(toolName)
	},
}
