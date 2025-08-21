package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainInstallCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install a CLI binary from the registry",
	Long: `Install a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version
Examples:
  toolchain install suzuki-shunsuke/github-comment@v3.5.0
  toolchain install hashicorp/terraform@v1.5.0
  toolchain install                    # Install from .tool-versions file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

func runInstall(cmd *cobra.Command, args []string) error {
	toolSpec := ""
	if len(args) > 0 {
		toolSpec = args[0]
	}
	return toolchain.RunInstall(toolSpec, defaultFlag, reinstallFlag)
}

var reinstallFlag bool
var defaultFlag bool

func init() {
	toolchainInstallCmd.Flags().BoolVar(&defaultFlag, "default", false, "Set installed version as default (front of .tool-versions)")
	toolchainInstallCmd.Flags().BoolVar(&reinstallFlag, "reinstall", false, "Reinstall even if already installed")
}
