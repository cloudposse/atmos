package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed tools and versions",
	Long:  `List all installed tools, versions, install date, and file size in human readable format.`,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	// Read .tool-versions file
	toolVersions, err := LoadToolVersions(".tool-versions")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .tool-versions file found in current directory")
		}
		return fmt.Errorf("error reading .tool-versions: %w", err)
	}

	if len(toolVersions.Tools) == 0 {
		fmt.Println("No tools installed.")
		return nil
	}

	// Print header
	fmt.Printf("%-20s %-15s %-20s %-10s\n", "TOOL", "VERSION", "INSTALL DATE", "SIZE")
	fmt.Println(strings.Repeat("-", 70))

	// Check each tool in .tool-versions to see if it's installed
	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}
		version := versions[0] // default version
		installer := NewInstaller()
		_, _, found := LookupToolVersion(toolName, toolVersions, installer.resolver)
		if !found {
			continue // skip if not found (shouldn't happen)
		}
		// Resolve tool name to owner/repo
		owner, repo, err := installer.resolver.Resolve(toolName)
		if err != nil {
			// Skip tools that can't be resolved
			continue
		}

		// If no version is specified, try to get the latest non-prerelease version
		if version == "" {
			registry := NewAquaRegistry()
			latestVersion, err := registry.GetLatestVersion(owner, repo)
			if err != nil {
				// Skip tools where we can't get the latest version
				continue
			}
			version = latestVersion
		}

		// Check if the tool is installed
		binaryPath, err := installer.findBinaryPath(owner, repo, version)
		if err != nil {
			// Tool not installed, skip it
			continue
		}

		// Get file info for size and date
		fileInfo, err := os.Stat(binaryPath)
		if err != nil {
			// Skip if we can't get file info
			continue
		}

		// Format file size
		size := formatFileSize(fileInfo.Size())

		// Format install date
		installDate := fileInfo.ModTime().Format("2006-01-02 15:04")

		// Get the binary name from the path
		binaryName := filepath.Base(binaryPath)

		// Resolve "latest" to actual version for display
		displayVersion := version
		if version == "latest" {
			actualVersion, err := installer.readLatestFile(owner, repo)
			if err == nil {
				displayVersion = actualVersion
			}
		}

		// Print formatted output
		fmt.Printf("%-20s %-15s %-20s %-10s\n", binaryName, displayVersion, installDate, size)
	}

	return nil
}

// formatFileSize formats file size in human readable format
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
