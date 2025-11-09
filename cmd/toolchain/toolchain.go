package toolchain

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	registrycmd "github.com/cloudposse/atmos/cmd/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	toolchainpkg "github.com/cloudposse/atmos/toolchain"
)

const (
	// GitHub token flag is the name of the GitHub token flag and configuration key.
	githubTokenFlag = "github-token"
)

var (
	githubToken      string
	toolVersionsFile string
	toolsDir         string
	toolsConfigFile  string
)

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

		// Initialize the toolchain package with the Atmos configuration.
		// This ensures that the toolchain package has access to the configuration.
		atmosCfg := &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				VersionsFile: toolVersionsFile,
				InstallPath:  toolsDir,
			},
		}

		// Call the toolchain package's SetAtmosConfig.
		toolchainpkg.SetAtmosConfig(atmosCfg)

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show help when no subcommands are provided.
		return cmd.Help()
	},
}

func init() {
	// Add GitHub token flag and bind to environment variables.
	toolchainCmd.PersistentFlags().StringVar(&githubToken, githubTokenFlag, "", "GitHub token for authenticated requests")
	if err := toolchainCmd.PersistentFlags().MarkHidden(githubTokenFlag); err != nil {
		fmt.Fprintf(os.Stderr, "Error hiding %s flag: %v\n", githubTokenFlag, err)
	}
	// Bind environment variables with proper precedence (ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN).
	if err := viper.BindPFlag(githubTokenFlag, toolchainCmd.PersistentFlags().Lookup(githubTokenFlag)); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding %s flag: %v\n", githubTokenFlag, err)
	}
	if err := viper.BindEnv(githubTokenFlag, "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding %s environment variables: %v\n", githubTokenFlag, err)
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
