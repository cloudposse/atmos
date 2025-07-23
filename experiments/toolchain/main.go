package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	logLevel string
)

var rootCmd = &cobra.Command{
	Use:   "toolchain",
	Short: "Install CLI binaries using registry metadata",
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
}

func main() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(toolVersionsCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(pathCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
