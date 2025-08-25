package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainUninstallCmd = &cobra.Command{
	Use:   "uninstall [tool]",
	Short: "Uninstall a CLI binary from the registry",
	Long: `Uninstall a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version or tool@version.
If no tool is specified, uninstalls all tools from the .tool-versions file.

Examples:
  toolchain uninstall terraform@1.9.8
  toolchain uninstall hashicorp/terraform@1.11.4
  toolchain uninstall                    # Uninstall all tools from .tool-versions`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUninstall,
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return toolchain.RunUninstall(args[0])
	}
	return toolchain.RunUninstall("")
}
