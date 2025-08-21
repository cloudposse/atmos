package cmd

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all installed tools by deleting the .tools directory",
	Long: `Remove all installed tools by deleting the .tools directory.

This command will:
- Count the number of files/directories in the .tools directory
- Delete the entire .tools directory and all its contents
- Display a summary of what was deleted

Use this command to completely clean up all installed tools.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		toolsDir := atmosConfig.Toolchain.ToolsDir

		homeDir, _ := os.UserHomeDir()
		cacheDir := filepath.Join(homeDir, ".cache", "tools-cache")
		tempCacheDir := filepath.Join(os.TempDir(), "tools-cache")

		return toolchain.CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir)
	},
}
