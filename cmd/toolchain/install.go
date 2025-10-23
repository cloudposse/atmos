package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var (
	reinstallFlag bool
	defaultFlag   bool
)

var installCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install a CLI binary from the registry",
	Long: `Install a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version
`,
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
