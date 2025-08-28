package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Emit the complete PATH environment variable for configured tool versions",
	Long: `Emit the complete PATH environment variable for configured tool versions.

This command reads the .tool-versions file and constructs a PATH that includes
all installed tool versions in the correct order for execution.

Examples:
  toolchain path                    # Print PATH for all tools in .tool-versions (absolute paths)
  toolchain path --relative         # Print PATH with relative paths
  toolchain path --export           # Print export PATH=... for shell sourcing
  toolchain path --json             # Print PATH as JSON object`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.EmitPath(exportFlag, jsonFlag, relativeFlag)
	},
}

var (
	exportFlag   bool
	jsonFlag     bool
	relativeFlag bool
)

func init() {
	toolchainPathCmd.Flags().BoolVar(&exportFlag, "export", false, "Print export PATH=... for shell sourcing")
	toolchainPathCmd.Flags().BoolVar(&jsonFlag, "json", false, "Print PATH as JSON object")
	toolchainPathCmd.Flags().BoolVar(&relativeFlag, "relative", false, "Use relative paths instead of absolute paths")
}
