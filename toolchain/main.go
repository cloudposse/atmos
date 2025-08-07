package toolchain

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set log level if specified
		if logLevel != "" {
			if err := SetLogLevel(logLevel); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	// Initialize the global logger
	InitLogger()

	// Add log level flag
	ToolChainCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, warn, error)")

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

// GetToolVersionsFilePath returns the path to the tool-versions file
func GetToolVersionsFilePath() string {
	return toolVersionsFile
}

// GetToolsDirPath returns the path to the tools directory
func GetToolsDirPath() string {
	return toolsDir
}

// GetToolsConfigFilePath returns the path to the tools configuration file
func GetToolsConfigFilePath() string {
	return toolsConfigFile
}

func init() {
	infoCmd.Flags().String("output", "table", "Output format: table or yaml")
	ToolChainCmd.AddCommand(addCmd)
	ToolChainCmd.AddCommand(removeCmd)
	ToolChainCmd.AddCommand(setCmd)
	ToolChainCmd.AddCommand(getCmd)
	ToolChainCmd.AddCommand(cleanCmd)
	// toolVersionsCmd removed - functionality merged into listCmd, but file kept as library
	ToolChainCmd.AddCommand(runCmd)
	ToolChainCmd.AddCommand(execCmd)
	ToolChainCmd.AddCommand(listCmd)
	ToolChainCmd.AddCommand(installCmd)
	ToolChainCmd.AddCommand(uninstallCmd)
	ToolChainCmd.AddCommand(pathCmd)
	ToolChainCmd.AddCommand(infoCmd)
	ToolChainCmd.AddCommand(aliasesCmd)
	ToolChainCmd.AddCommand(whichCmd)
}
