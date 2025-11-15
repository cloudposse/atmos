package toolchain

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/xdg"
	"github.com/cloudposse/atmos/toolchain"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean tools and cache directories",
	Long:  `Remove all installed tools and cached downloads.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		toolsDir := toolchain.GetInstallPath()

		// Use XDG-compliant cache directory.
		cacheDir, err := xdg.GetXDGCacheDir("atmos/toolchain", 0o755)
		if err != nil {
			// Fallback to legacy path if XDG fails.
			homeDir, _ := os.UserHomeDir()
			cacheDir = filepath.Join(homeDir, ".cache", "tools-cache")
		}

		tempCacheDir := filepath.Join(os.TempDir(), "atmos-toolchain-cache")
		return toolchain.CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir)
	},
}

// CleanCommandProvider implements the CommandProvider interface.
type CleanCommandProvider struct{}

func (c *CleanCommandProvider) GetCommand() *cobra.Command {
	return cleanCmd
}

func (c *CleanCommandProvider) GetName() string {
	return "clean"
}

func (c *CleanCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (c *CleanCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (c *CleanCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (c *CleanCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
