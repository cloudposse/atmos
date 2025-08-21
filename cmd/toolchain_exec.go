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

Examples:
  toolchain exec terraform --version          # Uses latest version
  toolchain exec terraform@1.9.8 --version   # Uses specific version
  toolchain exec opentofu@1.10.1 init
  toolchain exec terraform@1.5.7 plan -var-file=prod.tfvars`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		installer := toolchain.NewInstaller()
		return toolchain.RunExecCommand(installer, args)
	},
	DisableFlagParsing: true,
}
