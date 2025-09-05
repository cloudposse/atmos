package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainGetCmd = &cobra.Command{
	Use:   "versions <tool>",
	Short: "List available or configured versions for a tool",
	Args:  cobra.ExactArgs(1), // Requires exactly one argument (tool name)
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		showAll, _ := cmd.Flags().GetBool("all")
		limit, _ := cmd.Flags().GetInt("limit")
		toolName := args[0]
		if filePath != "" {
			atmosConfig.Toolchain.FilePath = filePath
		}
		toolchain.SetAtmosConfig(&atmosConfig)
		return toolchain.ListToolVersions(showAll, limit, toolName)
	},
}

func init() {
	toolchainGetCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
	toolchainGetCmd.Flags().Bool("all", false, "Fetch all available versions from GitHub API")
	toolchainGetCmd.Flags().Int("limit", 50, "Maximum number of versions to fetch when using --all")
}
