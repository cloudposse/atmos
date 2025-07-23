package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Emit the complete PATH environment variable for configured tool versions",
	Long: `Emit the complete PATH environment variable for configured tool versions.

This command reads the .tool-versions file and constructs a PATH that includes
all installed tool versions in the correct order for execution.

Examples:
  toolchain path                    # Print PATH for all tools in .tool-versions (absolute paths)
  toolchain path --relative         # Print PATH with relative paths
  toolchain path --export           # Print export PATH=... for shell sourcing
  toolchain path --json             # Print PATH as JSON object`,
	RunE: emitPath,
}

var (
	exportFlag   bool
	jsonFlag     bool
	relativeFlag bool
)

func init() {
	pathCmd.Flags().BoolVar(&exportFlag, "export", false, "Print export PATH=... for shell sourcing")
	pathCmd.Flags().BoolVar(&jsonFlag, "json", false, "Print PATH as JSON object")
	pathCmd.Flags().BoolVar(&relativeFlag, "relative", false, "Use relative paths instead of absolute paths")
}

func emitPath(cmd *cobra.Command, args []string) error {
	installer := NewInstaller()

	// Read .tool-versions file
	toolVersions, err := installer.loadToolVersions(".tool-versions")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .tool-versions file found in current directory")
		}
		return fmt.Errorf("error reading .tool-versions: %w", err)
	}

	if len(toolVersions.Tools) == 0 {
		return fmt.Errorf("no tools configured in .tool-versions file")
	}

	// Build PATH entries for each tool
	var pathEntries []string
	var toolPaths []ToolPath

	for toolName, version := range toolVersions.Tools {
		// Resolve tool name to owner/repo
		owner, repo, err := installer.resolveToolName(toolName)
		if err != nil {
			// Skip tools that can't be resolved
			continue
		}

		// Find the actual binary path (handles path inconsistencies)
		binaryPath, err := installer.findBinaryPath(owner, repo, version)
		if err != nil {
			// Tool not installed, skip it
			continue
		}

		var dirPath string
		if relativeFlag {
			// Use relative path
			dirPath = filepath.Dir(binaryPath)
		} else {
			// Convert to absolute path (default behavior)
			absBinaryPath, err := filepath.Abs(binaryPath)
			if err != nil {
				// Skip if we can't get absolute path
				continue
			}
			dirPath = filepath.Dir(absBinaryPath)
		}

		pathEntries = append(pathEntries, dirPath)
		toolPaths = append(toolPaths, ToolPath{
			Tool:    toolName,
			Version: version,
			Path:    dirPath,
		})
	}

	if len(pathEntries) == 0 {
		return fmt.Errorf("no installed tools found from .tool-versions")
	}

	// Sort for consistent output
	sort.Strings(pathEntries)
	sort.Slice(toolPaths, func(i, j int) bool {
		return toolPaths[i].Tool < toolPaths[j].Tool
	})

	// Get current PATH
	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		currentPath = "/usr/local/bin:/usr/bin:/bin"
	}

	// Construct final PATH
	finalPath := strings.Join(pathEntries, ":") + ":" + currentPath

	// Output based on flags
	if jsonFlag {
		return emitJSONPath(toolPaths, finalPath)
	} else if exportFlag {
		fmt.Printf("export PATH=\"%s\"\n", finalPath)
	} else {
		fmt.Println(finalPath)
	}

	return nil
}

// ToolPath represents a tool with its version and path
type ToolPath struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

func emitJSONPath(toolPaths []ToolPath, finalPath string) error {
	// Simple JSON output (you could use encoding/json for more complex cases)
	fmt.Printf("{\n")
	fmt.Printf("  \"tools\": [\n")
	for i, tool := range toolPaths {
		fmt.Printf("    {\n")
		fmt.Printf("      \"tool\": \"%s\",\n", tool.Tool)
		fmt.Printf("      \"version\": \"%s\",\n", tool.Version)
		fmt.Printf("      \"path\": \"%s\"\n", tool.Path)
		if i < len(toolPaths)-1 {
			fmt.Printf("    },\n")
		} else {
			fmt.Printf("    }\n")
		}
	}
	fmt.Printf("  ],\n")
	fmt.Printf("  \"final_path\": \"%s\",\n", finalPath)
	fmt.Printf("  \"count\": %d\n", len(toolPaths))
	fmt.Printf("}\n")

	return nil
}
