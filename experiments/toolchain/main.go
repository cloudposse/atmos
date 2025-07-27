package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	logLevel    string
	githubToken string
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

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Display the rendered YAML configuration for a tool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]

		// Create installer to access tool resolution
		installer := NewInstaller()

		// Parse tool name to get owner/repo
		owner, repo, err := installer.parseToolSpec(toolName)
		if err != nil {
			return fmt.Errorf("failed to parse tool name: %w", err)
		}

		// Find the tool configuration (use "latest" as default version)
		tool, err := installer.findTool(owner, repo, "latest")
		if err != nil {
			return fmt.Errorf("failed to find tool %s: %w", toolName, err)
		}

		// Display tool information in a more readable format
		cmd.Printf("Tool: %s\n", toolName)
		cmd.Printf("Owner/Repo: %s/%s\n", owner, repo)
		cmd.Printf("Type: %s\n", tool.Type)
		if tool.RepoOwner != "" {
			cmd.Printf("Repository: %s/%s\n", tool.RepoOwner, tool.RepoName)
		}
		if tool.Asset != "" {
			cmd.Printf("Asset Template: %s\n", tool.Asset)
		}
		if tool.Format != "" {
			cmd.Printf("Format: %s\n", tool.Format)
		}
		if tool.BinaryName != "" {
			cmd.Printf("Binary Name: %s\n", tool.BinaryName)
		}
		if len(tool.Files) > 0 {
			cmd.Printf("Files:\n")
			for _, file := range tool.Files {
				cmd.Printf("  - %s -> %s\n", file.Src, file.Name)
			}
		}
		if len(tool.Overrides) > 0 {
			cmd.Printf("Overrides:\n")
			for _, override := range tool.Overrides {
				cmd.Printf("  - %s/%s: %s\n", override.GOOS, override.GOARCH, override.Asset)
			}
		}

		// Also show the raw YAML for debugging
		cmd.Printf("\nRaw YAML Configuration:\n")
		yamlData, err := toolToYAML(tool)
		if err != nil {
			return fmt.Errorf("failed to convert tool to YAML: %w", err)
		}
		cmd.Printf("%s\n", yamlData)

		return nil
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all installed tools by deleting the .tools directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		toolsDir := ".tools"
		count := 0
		err := filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path != toolsDir {
				count++
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to count files in %s: %w", toolsDir, err)
		}
		err = os.RemoveAll(toolsDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", toolsDir, err)
		}
		cmd.Printf("%s Deleted %d files/directories from %s\n", checkMark.Render(), count, toolsDir)
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

	// Set default value from environment variables (ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN)
	if token := os.Getenv("ATMOS_GITHUB_TOKEN"); token != "" {
		rootCmd.PersistentFlags().Set("github-token", token)
	} else if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		rootCmd.PersistentFlags().Set("github-token", token)
	}

	rootCmd.AddCommand(cleanCmd)
}

func main() {
	addCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
	removeCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(toolVersionsCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(pathCmd)
	rootCmd.AddCommand(infoCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
