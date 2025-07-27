package main

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
)

var rootCmd = &cobra.Command{
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
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, warn, error)")

	// Add GitHub token flag and bind to environment variables
	rootCmd.PersistentFlags().StringVar(&githubToken, "github-token", "", "GitHub token for authenticated requests")
	rootCmd.PersistentFlags().MarkHidden("github-token") // Hide from help since it's primarily for env vars
	// Bind environment variables with proper precedence (ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN)
	if err := viper.BindPFlag("github-token", rootCmd.PersistentFlags().Lookup("github-token")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token flag: %v\n", err)
	}
	if err := viper.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding github-token environment variables: %v\n", err)
	}

	// Add tool-versions file flag
	rootCmd.PersistentFlags().StringVar(&toolVersionsFile, "tool-versions", ".tool-versions", "Path to tool-versions file")

	// Add tools directory flag
	rootCmd.PersistentFlags().StringVar(&toolsDir, "tools-dir", ".tools", "Directory to store installed tools")
}

// GetToolVersionsFilePath returns the path to the tool-versions file
func GetToolVersionsFilePath() string {
	return toolVersionsFile
}

// GetToolsDirPath returns the path to the tools directory
func GetToolsDirPath() string {
	return toolsDir
}

func main() {
	infoCmd.Flags().String("output", "table", "Output format: table or yaml")
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(toolVersionsCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(pathCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(aliasesCmd)
	rootCmd.AddCommand(whichCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
