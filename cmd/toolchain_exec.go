package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainExecCmd = &cobra.Command{
	Use:   "exec [tool[@version]] [flags...]",
	Short: "Exec a specific version of a tool (replaces current process)",
	Long: `Exec a specific version of a tool with arguments, replacing the current process.

If no version is specified, the latest version will be used.
`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		installer := toolchain.NewInstaller()
		return toolchain.RunExecCommand(installer, args)
	},
	DisableFlagParsing: true,
}
