package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var execCmd = &cobra.Command{
	Use:   "exec <tool@version> [args...]",
	Short: "Execute a tool with specified version",
	Long:  `Execute a tool with a specific version, installing it if necessary.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		installer := toolchain.NewInstaller()
		return toolchain.RunExecCommand(installer, args)
	},
}
