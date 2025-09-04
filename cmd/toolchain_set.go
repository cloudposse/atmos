package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainSetCmd = &cobra.Command{
	Use:   "set <tool> [version]",
	Short: "Set a specific version for a tool in .tool-versions",
	Long: `Set a specific version for a tool in the .tool-versions file.

If no version is provided, this command will fetch available versions from GitHub releases
and present them in an interactive selection (only works for github_release type tools).

Examples:
atmos toolchain set terraform 1.11.4
atmos toolchain set hashicorp/terraform 1.11.4
atmos toolchain set terraform  # Interactive version selection
atmos toolchain set --file /path/to/.tool-versions kubectl 1.28.0`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		scrollSpeed, _ := cmd.Flags().GetInt("scroll-speed")
		if filePath != "" {
			atmosConfig.Toolchain.FilePath = filePath
		}

		toolName := args[0]
		version := ""
		if len(args) > 1 {
			version = args[1]
		}

		return toolchain.SetToolVersion(toolName, version, scrollSpeed)
	},
}

func init() {
	toolchainSetCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
	toolchainSetCmd.Flags().Int("scroll-speed", 3, "Scroll speed multiplier for viewport (1=normal, 2=fast, 3=very fast, etc.)")
}
