package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logLevel         string
	githubToken      string
	toolVersionsFile string
	toolsDir         string
	toolsConfigFile  string
)

var ToolChainCmd = &cobra.Command{
	Use:   "toolchain",
	Short: "Toolchain CLI",
	Long:  `A standalone tool to install CLI binaries using registry metadata.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Set log level
		return nil
	},
}

func init() {
	// Add GitHub token flag and bind to environment variables
	ToolChainCmd.PersistentFlags().StringVar(&githubToken, "github-token", "", "GitHub token for authenticated requests")
	ToolChainCmd.PersistentFlags().MarkHidden("github-token") // Hide from help since it's primarily for env vars
	// Bind environment variables with proper precedence (ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN)
	if err := viper.BindPFlag("github-token", ToolChainCmd.PersistentFlags().Lookup("github-token")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token flag: %v\n", err)
	}
	if err := viper.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token environment variables: %v\n", err)
	}

	// Add tool-versions file flagd
	ToolChainCmd.PersistentFlags().StringVar(&toolVersionsFile, "tool-versions", ".tool-versions", "Path to tool-versions file")

	// Add tools directory flag
	ToolChainCmd.PersistentFlags().StringVar(&toolsDir, "tools-dir", ".tools", "Directory to store installed tools")

	// Add tools config file flag
	ToolChainCmd.PersistentFlags().StringVar(&toolsConfigFile, "tools-config", "tools.yaml", "Path to tools configuration file")
}

func init() {
	ToolChainCmd.AddCommand(toolchainAddCmd)
	ToolChainCmd.AddCommand(toolchainRemoveCmd)
	ToolChainCmd.AddCommand(toolchainSetCmd)
	ToolChainCmd.AddCommand(toolchainVersionsCmd)
	ToolChainCmd.AddCommand(toolchainCleanCmd)
	ToolChainCmd.AddCommand(toolchainExecCmd)
	ToolChainCmd.AddCommand(toolchainListCmd)
	ToolChainCmd.AddCommand(toolchainInstallCmd)
	ToolChainCmd.AddCommand(toolchainUninstallCmd)
	ToolChainCmd.AddCommand(toolchainPathCmd)
	ToolChainCmd.AddCommand(toolchainInfoCmd)
	ToolChainCmd.AddCommand(toolchainAliasesCmd)
	ToolChainCmd.AddCommand(toolchainWhichCmd)
}
