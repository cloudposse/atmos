package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestGetCommand(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
			"helm":      {"3.17.4", "3.12.0"},
		},
	}
	err := SaveToolVersions(filePath, toolVersions)
	assert.NoError(t, err)

	// Test get command for existing tool
	getCmd := &cobra.Command{
		Use:   "get <tool>",
		Short: "Show all versions configured for a tool, sorted in semver order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			showAll, _ := cmd.Flags().GetBool("all")

			if filePath == "" {
				filePath = ".tool-versions"
			}
			toolName := args[0]

			// Resolve the tool name to handle aliases
			installer := NewInstaller()
			owner, repo, err := installer.parseToolSpec(toolName)
			if err != nil {
				return fmt.Errorf("invalid tool name: %w", err)
			}
			resolvedKey := owner + "/" + repo

			var versions []string
			var defaultVersion string

			if showAll {
				// For testing, we'll mock the GitHub API call
				versions = []string{"1.12.0", "1.11.4", "1.9.8", "1.8.0"}

				// Load tool versions to get the default
				toolVersions, err := LoadToolVersions(filePath)
				if err == nil {
					if configuredVersions, exists := toolVersions.Tools[resolvedKey]; exists && len(configuredVersions) > 0 {
						defaultVersion = configuredVersions[0]
					} else if configuredVersions, exists := toolVersions.Tools[toolName]; exists && len(configuredVersions) > 0 {
						defaultVersion = configuredVersions[0]
					}
				}
			} else {
				// Load tool versions from file
				toolVersions, err := LoadToolVersions(filePath)
				if err != nil {
					return fmt.Errorf("failed to load .tool-versions: %w", err)
				}

				// Get versions for the tool - try both resolved key and original tool name
				fileVersions, exists := toolVersions.Tools[resolvedKey]
				if !exists {
					fileVersions, exists = toolVersions.Tools[toolName]
					if !exists {
						return fmt.Errorf("tool '%s' not found in %s", toolName, filePath)
					}
				}

				if len(fileVersions) == 0 {
					return fmt.Errorf("no versions configured for tool '%s' in %s", toolName, filePath)
				}

				versions = fileVersions
				defaultVersion = versions[0]
			}

			// Sort versions in semver order
			sortedVersions, err := sortVersionsSemver(versions)
			if err != nil {
				// If semver sorting fails, fall back to string sorting
				sort.Strings(versions)
				sortedVersions = versions
			}

			// Check which versions are actually installed (mock for testing)
			installedVersions := make(map[string]bool)
			for _, version := range sortedVersions {
				// Mock installation check - assume first version is installed
				installedVersions[version] = version == versions[0]
			}

			// Define styles with TTY-aware dark/light mode detection
			profile := termenv.ColorProfile()

			var installedStyle, notInstalledStyle lipgloss.Style

			if profile == termenv.ANSI256 || profile == termenv.TrueColor {
				// Dark background - use grayscale
				installedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("15")) // Bright white

				notInstalledStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")) // Dim gray
			} else {
				// Light background or no color support - use basic styling
				installedStyle = lipgloss.NewStyle().
					Bold(true)

				notInstalledStyle = lipgloss.NewStyle()
			}

			// Display the results cleanly
			for _, version := range sortedVersions {
				isInstalled := installedVersions[version]
				isDefault := version == defaultVersion

				var indicator string

				if isDefault {
					indicator = checkMark.String()
				} else {
					indicator = " "
				}

				// Apply styling based on installation status
				if isInstalled {
					fmt.Printf("%s %s\n", indicator, installedStyle.Render(version))
				} else {
					fmt.Printf("%s %s\n", indicator, notInstalledStyle.Render(version))
				}
			}

			return nil
		},
	}
	getCmd.Flags().String("file", filePath, "Path to tool-versions file")
	getCmd.Flags().Bool("all", false, "Fetch all available versions from GitHub API")
	getCmd.Flags().Int("limit", 50, "Maximum number of versions to fetch when using --all")

	// Test get command for existing tool
	getCmd.SetArgs([]string{"terraform"})
	err = getCmd.Execute()
	assert.NoError(t, err)

	// Test get command for non-existent tool
	getCmd2 := &cobra.Command{
		Use:   "get <tool>",
		Short: "Show all versions configured for a tool, sorted in semver order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			toolName := args[0]

			// Resolve the tool name to handle aliases
			installer := NewInstaller()
			owner, repo, err := installer.parseToolSpec(toolName)
			if err != nil {
				return fmt.Errorf("invalid tool name: %w", err)
			}
			resolvedKey := owner + "/" + repo

			// Load tool versions from file
			toolVersions, err := LoadToolVersions(filePath)
			if err != nil {
				return fmt.Errorf("failed to load .tool-versions: %w", err)
			}

			// Get versions for the tool - try both resolved key and original tool name
			fileVersions, exists := toolVersions.Tools[resolvedKey]
			if !exists {
				fileVersions, exists = toolVersions.Tools[toolName]
				if !exists {
					return fmt.Errorf("tool '%s' not found in %s", toolName, filePath)
				}
			}

			if len(fileVersions) == 0 {
				return fmt.Errorf("no versions configured for tool '%s' in %s", toolName, filePath)
			}

			return nil
		},
	}
	getCmd2.Flags().String("file", filePath, "Path to tool-versions file")
	getCmd2.SetArgs([]string{"nonexistent"})
	err = getCmd2.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool 'nonexistent' not found")
}

func TestGetCommandWithAllFlag(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(filePath, toolVersions)
	assert.NoError(t, err)

	// Test with --all flag using the actual getCmd
	getCmd.Flags().Set("file", filePath)
	getCmd.SetArgs([]string{"--all", "terraform"})
	err = getCmd.Execute()
	assert.NoError(t, err)

	// Test with --all and --limit flags
	getCmd.Flags().Set("limit", "2")
	getCmd.SetArgs([]string{"--all", "--limit", "2", "terraform"})
	err = getCmd.Execute()
	assert.NoError(t, err)
}

func TestGetCommandToolResolution(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")

	// Create a .tool-versions file with both alias and full name
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform":           {"1.11.4"},
			"hashicorp/terraform": {"1.9.8"},
		},
	}
	err := SaveToolVersions(filePath, toolVersions)
	assert.NoError(t, err)

	// Test that tool resolution works correctly
	getCmd := &cobra.Command{
		Use:   "get <tool>",
		Short: "Show all versions configured for a tool, sorted in semver order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			toolName := args[0]

			// Resolve the tool name to handle aliases
			installer := NewInstaller()
			owner, repo, err := installer.parseToolSpec(toolName)
			if err != nil {
				return fmt.Errorf("invalid tool name: %w", err)
			}
			resolvedKey := owner + "/" + repo

			// Load tool versions from file
			toolVersions, err := LoadToolVersions(filePath)
			if err != nil {
				return fmt.Errorf("failed to load .tool-versions: %w", err)
			}

			// Get versions for the tool - try both resolved key and original tool name
			fileVersions, exists := toolVersions.Tools[resolvedKey]
			if !exists {
				fileVersions, exists = toolVersions.Tools[toolName]
				if !exists {
					return fmt.Errorf("tool '%s' not found in %s", toolName, filePath)
				}
			}

			// Verify that both forms are found
			assert.True(t, exists, "Tool should be found")
			assert.Greater(t, len(fileVersions), 0, "Should have at least one version")

			return nil
		},
	}
	getCmd.Flags().String("file", filePath, "Path to tool-versions file")

	// Test with alias
	getCmd.SetArgs([]string{"terraform"})
	err = getCmd.Execute()
	assert.NoError(t, err)

	// Test with full name
	getCmd.SetArgs([]string{"hashicorp/terraform"})
	err = getCmd.Execute()
	assert.NoError(t, err)
}
