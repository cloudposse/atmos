package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var toolVersionsCmd = &cobra.Command{
	Use:   "tool-versions [file]",
	Short: "List tools from .tool-versions file and their install status",
	Long: `List all tools specified in a .tool-versions file and show their install status.

Examples:
  toolchain tool-versions                    # Use .tool-versions in current directory
  toolchain tool-versions .tool-versions     # Use specific file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runToolVersions,
}

func runToolVersions(cmd *cobra.Command, args []string) error {
	filePath := ".tool-versions"
	if len(args) > 0 {
		filePath = args[0]
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Load tool versions
	toolVersions, err := loadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	installer := NewInstaller()

	fmt.Printf("📋 Tools from %s:\n", filePath)
	fmt.Printf("%-30s %-15s\n", "Tool", "Version")
	fmt.Printf("%s\n", strings.Repeat("-", 50))

	for tool, version := range toolVersions.Tools {
		// Parse tool specification (owner/repo@version or just repo@version)
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			fmt.Printf("%-30s %-15s %s\n", tool, version, xMark.Render())
			continue
		}

		// Check if installed
		_, err = installer.findBinaryPath(owner, repo, version)
		status := xMark.Render()
		if err == nil {
			status = checkMark.Render()
		}

		fmt.Printf("%-30s %-15s %s\n", tool, version, status)
	}

	return nil
}

// loadToolVersions loads a .tool-versions file
func loadToolVersions(filePath string) (*ToolVersions, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	toolVersions := &ToolVersions{
		Tools: make(map[string]string),
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			toolVersions.Tools[parts[0]] = parts[1]
		}
	}

	return toolVersions, nil
}
