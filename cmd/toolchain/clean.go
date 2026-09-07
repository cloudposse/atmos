package toolchain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/xdg"
)

var cleanParser *flags.StandardParser

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean tools and cache directories",
	Long: `Remove all installed tools and cached downloads.

By default this prompts for confirmation before deleting anything (use --force to skip the
prompt, or --dry-run to preview what would be deleted without removing anything).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := cleanParser.BindFlagsToViper(cmd, v); err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrFlagBinding, err)
		}

		toolsDir := toolchain.GetInstallPath()

		// Use XDG-compliant cache directory.
		// Note: GetXDGCacheDir already prepends "atmos/" to the subpath.
		cacheDir, err := xdg.GetXDGCacheDir("toolchain", 0o755)
		if err != nil {
			// Fallback to legacy path if XDG fails.
			homeDir, homeErr := homedir.Dir()
			if homeErr != nil {
				return fmt.Errorf("could not determine home directory: %w", homeErr)
			}
			cacheDir = filepath.Join(homeDir, ".cache", "tools-cache")
		}

		tempCacheDir := filepath.Join(os.TempDir(), "atmos-toolchain-cache")

		opts := toolchain.CleanOptions{
			DryRun:    v.GetBool("dry-run"),
			CacheOnly: v.GetBool("cache-only"),
			Force:     v.GetBool("force"),
		}

		return toolchain.RunClean(toolsDir, cacheDir, tempCacheDir, opts)
	},
}

func init() {
	// Create parser with clean-specific flags.
	cleanParser = flags.NewStandardParser(
		flags.WithBoolFlag("dry-run", "", false, "Show what would be cleaned without actually removing anything"),
		flags.WithBoolFlag("cache-only", "", false, "Only clean the download cache, not installed tools"),
		flags.WithBoolFlag("force", "", false, "Skip the confirmation prompt and immediately clean"),
		flags.WithEnvVars("dry-run", "ATMOS_TOOLCHAIN_DRY_RUN"),
		flags.WithEnvVars("cache-only", "ATMOS_TOOLCHAIN_CACHE_ONLY"),
		flags.WithEnvVars("force", "ATMOS_TOOLCHAIN_FORCE"),
	)

	// Register flags.
	cleanParser.RegisterFlags(cleanCmd)

	// Bind flags to Viper.
	if err := cleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
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
	return cleanParser
}

func (c *CleanCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (c *CleanCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
