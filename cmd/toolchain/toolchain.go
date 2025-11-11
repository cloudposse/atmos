package toolchain

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	registrycmd "github.com/cloudposse/atmos/cmd/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	toolchainpkg "github.com/cloudposse/atmos/toolchain"
)

// ToolchainParser handles flag parsing for toolchain persistent flags.
var toolchainParser *flags.StandardParser

// SetAtmosConfig sets the Atmos configuration for the toolchain command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	// Forward the configuration to the toolchain package.
	// This ensures the toolchain package has access to the Atmos configuration.
	toolchainpkg.SetAtmosConfig(config)
}

// toolchainCmd represents the toolchain command.
var toolchainCmd = &cobra.Command{
	Use:   "toolchain",
	Short: "Toolchain CLI",
	Long:  `A standalone tool to install CLI binaries using registry metadata.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize I/O context and global formatter (required for ui.* functions).
		// This must happen before any ui.* calls.
		ioCtx, ioErr := iolib.NewContext()
		if ioErr != nil {
			return fmt.Errorf("failed to initialize I/O context: %w", ioErr)
		}
		ui.InitFormatter(ioCtx)
		data.InitWriter(ioCtx)

		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := toolchainParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Update the toolchain configuration with flag/env overrides only.
		// Get the existing config (set by root.go Execute()) to preserve all settings.
		atmosCfg := toolchainpkg.GetAtmosConfig()
		if atmosCfg == nil {
			// Fallback: create new config if not initialized (shouldn't happen in normal flow).
			atmosCfg = &schema.AtmosConfiguration{}
		}

		// Apply overrides for fields that were explicitly set via CLI flags or environment variables.
		// This preserves important config like UseToolVersions, UseLockFile, and Registries
		// from atmos.yaml that was already loaded in root.go Execute().
		//
		// We apply values from viper (which has precedence: flag > env > config > default)
		// unconditionally because:
		// 1. The config from root.go already has values from atmos.yaml
		// 2. If viper returns a different value, it's because of a flag or env var override
		// 3. If viper returns the same value, we're just setting it to itself (no-op)
		atmosCfg.Toolchain.VersionsFile = v.GetString("toolchain.tool-versions")
		atmosCfg.Toolchain.InstallPath = v.GetString("toolchain.path")
		atmosCfg.Toolchain.ToolsDir = v.GetString("toolchain.path")

		// Update the toolchain package's config (no-op if we got it from there).
		toolchainpkg.SetAtmosConfig(atmosCfg)

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show help when no subcommands are provided.
		return cmd.Help()
	},
}

func init() {
	// Create parser with toolchain-specific persistent flags using functional options.
	toolchainParser = flags.NewStandardParser(
		flags.WithStringFlag("github-token", "", "", "GitHub token for authenticated requests"),
		flags.WithStringFlag("tool-versions", "", ".tool-versions", "Path to tool-versions file"),
		flags.WithStringFlag("toolchain-path", "", ".tools", "Directory to store installed tools"),
		flags.WithEnvVars("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"),
		flags.WithEnvVars("tool-versions", "ATMOS_TOOL_VERSIONS"),
		flags.WithEnvVars("toolchain-path", "ATMOS_TOOLCHAIN_PATH"),
	)

	// Register persistent flags (inherited by all subcommands).
	toolchainParser.RegisterPersistentFlags(toolchainCmd)

	// Hide the github-token flag from help.
	if err := toolchainCmd.PersistentFlags().MarkHidden("github-token"); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	// We need custom Viper keys for toolchain flags, so bind them manually.
	v := viper.GetViper()
	if err := v.BindPFlag("github-token", toolchainCmd.PersistentFlags().Lookup("github-token")); err != nil {
		panic(err)
	}
	if err := v.BindPFlag("toolchain.tool-versions", toolchainCmd.PersistentFlags().Lookup("tool-versions")); err != nil {
		panic(err)
	}
	if err := v.BindPFlag("toolchain.path", toolchainCmd.PersistentFlags().Lookup("toolchain-path")); err != nil {
		panic(err)
	}

	// Add all subcommands.
	toolchainCmd.AddCommand(addCmd)
	toolchainCmd.AddCommand(cleanCmd)
	toolchainCmd.AddCommand(duCmd)
	toolchainCmd.AddCommand(envCmd)
	toolchainCmd.AddCommand(execCmd)
	toolchainCmd.AddCommand(getCmd)
	toolchainCmd.AddCommand(infoCmd)
	toolchainCmd.AddCommand(installCmd)
	toolchainCmd.AddCommand(listCmd)
	toolchainCmd.AddCommand(pathCmd)
	toolchainCmd.AddCommand(registrycmd.GetRegistryCommand())
	toolchainCmd.AddCommand(removeCmd)
	toolchainCmd.AddCommand(searchAliasCmd)
	toolchainCmd.AddCommand(setCmd)
	toolchainCmd.AddCommand(uninstallCmd)
	toolchainCmd.AddCommand(whichCmd)

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ToolchainCommandProvider{})
}

// ToolchainCommandProvider implements the CommandProvider interface.
type ToolchainCommandProvider struct{}

// GetCommand returns the root toolchain command (*cobra.Command) for registration with the command registry.
// This command serves as the parent for all toolchain subcommands (add, install, list, etc.).
func (t *ToolchainCommandProvider) GetCommand() *cobra.Command {
	defer perf.Track(nil, "ToolchainCommandProvider.GetCommand")()

	return toolchainCmd
}

// GetName returns the unique command name ("toolchain") used for command registration and identification.
func (t *ToolchainCommandProvider) GetName() string {
	defer perf.Track(nil, "ToolchainCommandProvider.GetName")()

	return "toolchain"
}

// GetGroup returns the command group identifier ("Toolchain Commands") used for organizing commands in help output.
func (t *ToolchainCommandProvider) GetGroup() string {
	defer perf.Track(nil, "ToolchainCommandProvider.GetGroup")()

	return "Toolchain Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
func (t *ToolchainCommandProvider) GetFlagsBuilder() flags.Builder {
	return toolchainParser
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (t *ToolchainCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (t *ToolchainCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
