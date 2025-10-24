package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

func EmitPath(exportFlag, jsonFlag, relativeFlag bool) error {
	defer perf.Track(nil, "toolchain.PathExec")()

	installer := NewInstaller()

	// Read tool-versions file
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: no tools configured in tool-versions file", ErrToolNotFound)
		}
		return fmt.Errorf("error reading tool-versions file: %w", err)
	}

	if len(toolVersions.Tools) == 0 {
		return fmt.Errorf("%w: no tools installed from .tool-versions file", ErrToolNotFound)
	}

	// Build PATH entries for each tool
	var pathEntries []string
	var toolPaths []ToolPath

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}
		version := versions[0] // default version
		// Resolve tool name to owner/repo
		owner, repo, err := installer.resolver.Resolve(toolName)
		if err != nil {
			// Skip tools that can't be resolved
			continue
		}

		// Find the actual binary path (handles path inconsistencies)
		binaryPath, err := installer.FindBinaryPath(owner, repo, version)
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
		return fmt.Errorf("%w: no installed tools found from tool-versions file", ErrToolNotFound)
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
	switch {
	case jsonFlag:
		return emitJSONPath(toolPaths, finalPath)
	case exportFlag:
		fmt.Printf("export PATH=\"%s\"\n", finalPath)
	default:
		fmt.Println(finalPath)
	}

	return nil
}

// ToolPath represents a tool with its version and path.
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
