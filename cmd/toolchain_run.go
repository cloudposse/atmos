package cmd

import (
	"fmt"

	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainRunCmd = &cobra.Command{
	Use:   "run [tool[@version]] [flags...]",
	Short: "Run a specific version of a tool",
	Long: `Run a specific version of a tool with arguments.

If no version is specified, the latest version will be used.

Examples:
atmos toolchain run terraform --version          # Uses latest version
atmos toolchain run terraform@1.9.8 --version   # Uses specific version
atmos toolchain run opentofu@1.10.1 init
atmos toolchain run terraform@1.5.7 plan -var-file=prod.tfvars`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runTool,
	DisableFlagParsing: true,
}

func runTool(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided. Expected format: tool@version")
	}
	toolSpec := args[0]
	tool, version, err := toolchain.ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" {
		return fmt.Errorf("invalid tool specification: missing tool name")
	}
	remainingArgs := args[1:]

	installer := toolchain.NewInstaller()
	return toolchain.RunToolWithInstaller(installer, tool, version, remainingArgs)
}
