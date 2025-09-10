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
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUninstall,
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return toolchain.RunUninstall(args[0])
	}
	return toolchain.RunUninstall("")
}
