package toolchain

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	registrycmd "github.com/cloudposse/atmos/cmd/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainpkg "github.com/cloudposse/atmos/pkg/toolchain"
)

const (
	// Flag names.
	flagGitHubToken   = "github-token"
	flagToolVersions  = "tool-versions"
	flagToolchainPath = "toolchain-path"
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
	Short: "Manage tool versions and installations",
	Long:  `A standalone tool to install CLI binaries using registry metadata.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Call root command's PersistentPreRun first to ensure root command initialization runs.
		// This includes config loading, I/O initialization, and experimental command checks.
		// Without this, the toolchain command would bypass the experimental mode handling.
		// We use cmd.Root() because cmd.Parent() would return the toolchain command (not root)
		// when running subcommands like "toolchain list".
		if root := cmd.Root(); root != nil && root.PersistentPreRun != nil {
			root.PersistentPreRun(cmd, args)
		}

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

		// Only explicit command-line or environment overrides may replace the
		// initialized configuration. Applying flag defaults here used to make
		// `atmos toolchain` use .tools while automatic command dependencies used
		// the XDG cache default, so the two paths could disagree about what was
		// installed.
		if _, envSet := os.LookupEnv("ATMOS_TOOL_VERSIONS"); envSet || cmd.Flags().Changed(flagToolVersions) {
			atmosCfg.Toolchain.VersionsFile = v.GetString("toolchain.tool-versions")
		}
		if _, envSet := os.LookupEnv("ATMOS_TOOLCHAIN_PATH"); envSet || cmd.Flags().Changed(flagToolchainPath) {
			path := v.GetString("toolchain.path")
			atmosCfg.Toolchain.InstallPath = path
			atmosCfg.Toolchain.ToolsDir = path
		}

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
		flags.WithStringFlag(flagGitHubToken, "", "", "GitHub token for authenticated requests"),
		flags.WithStringFlag(flagToolVersions, "", ".tool-versions", "Path to tool-versions file"),
		flags.WithStringFlag(flagToolchainPath, "", ".tools", "Directory to store installed tools"),
		// Prefer the Atmos Pro-brokered token, then ATMOS_GITHUB_TOKEN, then GITHUB_TOKEN.
		// This matches the go-getter token injector and pkg/http GetGitHubTokenFromEnv so
		// toolchain installs use the same token as every other GitHub-talking path.
		flags.WithEnvVars(flagGitHubToken, "ATMOS_PRO_GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"),
		flags.WithEnvVars(flagToolVersions, "ATMOS_TOOL_VERSIONS"),
		flags.WithEnvVars(flagToolchainPath, "ATMOS_TOOLCHAIN_PATH"),
	)

	// Register persistent flags (inherited by all subcommands).
	toolchainParser.RegisterPersistentFlags(toolchainCmd)

	// Hide the github-token flag from help.
	if err := toolchainCmd.PersistentFlags().MarkHidden(flagGitHubToken); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	// We need custom Viper keys for toolchain flags, so bind them manually.
	v := viper.GetViper()
	if err := v.BindPFlag(flagGitHubToken, toolchainCmd.PersistentFlags().Lookup(flagGitHubToken)); err != nil {
		panic(err)
	}
	if err := v.BindPFlag("toolchain.tool-versions", toolchainCmd.PersistentFlags().Lookup(flagToolVersions)); err != nil {
		panic(err)
	}
	if err := v.BindPFlag("toolchain.path", toolchainCmd.PersistentFlags().Lookup(flagToolchainPath)); err != nil {
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

// GetAliases returns command aliases for the toolchain command.
func (t *ToolchainCommandProvider) GetAliases() []internal.CommandAlias {
	defer perf.Track(nil, "ToolchainCommandProvider.GetAliases")()

	return nil
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

// IsExperimental returns whether this command is experimental.
func (t *ToolchainCommandProvider) IsExperimental() bool {
	return true
}
