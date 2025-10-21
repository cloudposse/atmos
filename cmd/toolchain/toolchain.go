package toolchain

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	githubToken      string
	toolVersionsFile string
	toolsDir         string
	toolsConfigFile  string
)

// SetAtmosConfig sets the Atmos configuration for the toolchain command.
// This is called from root.go after atmosConfig is initialized.
// Currently unused but kept for future expansion when subcommands need access to config.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	// Reserved for future use when toolchain subcommands need access to Atmos configuration.
	_ = config
}

// toolchainCmd represents the toolchain command.
var toolchainCmd = &cobra.Command{
	Use:   "toolchain",
	Short: "Toolchain CLI",
	Long:  `A standalone tool to install CLI binaries using registry metadata.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Set log level.
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show help when no subcommands are provided.
		return cmd.Help()
	},
}

func init() {
	// Add GitHub token flag and bind to environment variables.
	toolchainCmd.PersistentFlags().StringVar(&githubToken, "github-token", "", "GitHub token for authenticated requests")
	toolchainCmd.PersistentFlags().MarkHidden("github-token") // Hide from help since it's primarily for env vars.
	// Bind environment variables with proper precedence (ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN).
	if err := viper.BindPFlag("github-token", toolchainCmd.PersistentFlags().Lookup("github-token")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token flag: %v\n", err)
	}
	if err := viper.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token environment variables: %v\n", err)
	}

	// Add tool-versions file flag.
	toolchainCmd.PersistentFlags().StringVar(&toolVersionsFile, "tool-versions", ".tool-versions", "Path to tool-versions file")

	// Add tools directory flag.
	toolchainCmd.PersistentFlags().StringVar(&toolsDir, "tools-dir", ".tools", "Directory to store installed tools")

	// Add tools config file flag.
	toolchainCmd.PersistentFlags().StringVar(&toolsConfigFile, "tools-config", "tools.yaml", "Path to tools configuration file")

	// Add all subcommands.
	toolchainCmd.AddCommand(addCmd)
	toolchainCmd.AddCommand(cleanCmd)
	toolchainCmd.AddCommand(execCmd)
	toolchainCmd.AddCommand(getCmd)
	toolchainCmd.AddCommand(infoCmd)
	toolchainCmd.AddCommand(installCmd)
	toolchainCmd.AddCommand(listCmd)
	toolchainCmd.AddCommand(pathCmd)
	toolchainCmd.AddCommand(removeCmd)
	toolchainCmd.AddCommand(setCmd)
	toolchainCmd.AddCommand(uninstallCmd)
	toolchainCmd.AddCommand(whichCmd)

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ToolchainCommandProvider{})
}

// ToolchainCommandProvider implements the CommandProvider interface.
type ToolchainCommandProvider struct{}

// GetCommand returns the toolchain command.
func (t *ToolchainCommandProvider) GetCommand() *cobra.Command {
	return toolchainCmd
}

// GetName returns the command name.
func (t *ToolchainCommandProvider) GetName() string {
	return "toolchain"
}

// GetGroup returns the command group for help organization.
func (t *ToolchainCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}
