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

var addCmd = &cobra.Command{
	Use:   "add <tool> <version>",
	Short: "Add or update a tool and version in .tool-versions",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = ".tool-versions"
		}
		tool := args[0]
		version := args[1]
		err := AddToolToVersions(filePath, tool, version)
		if err != nil {
			return err
		}
		cmd.Printf("%s Added/updated %s %s in %s\n", checkMark.Render(), tool, version, filePath)
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove <tool>",
	Short: "Remove a tool from .tool-versions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = ".tool-versions"
		}
		tool := args[0]
		err := RemoveToolFromVersions(filePath, tool)
		if err != nil {
			return err
		}
		cmd.Printf("%s Removed %s from %s\n", checkMark.Render(), tool, filePath)
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
	addCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
	removeCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(toolVersionsCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(pathCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
