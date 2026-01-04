package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/data"
)

// buildPathEntries constructs PATH entries from tool versions.
func buildPathEntries(toolVersions *ToolVersions, installer *Installer, relativeFlag bool) ([]string, []ToolPath, error) {
	var pathEntries []string
	var toolPaths []ToolPath
	seen := make(map[string]struct{}) // Track seen paths to avoid duplicates.

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}

		version := versions[0] // Default version.

		// Resolve tool name to owner/repo using parseToolSpec for consistency.
		owner, repo, err := installer.parseToolSpec(toolName)
		if err != nil {
			// Skip tools that can't be resolved.
			continue
		}

		// Find the actual binary path (handles path inconsistencies).
		binaryPath, err := installer.FindBinaryPath(owner, repo, version)
		if err != nil {
			// Tool not installed, skip it.
			continue
		}

		dirPath, err := resolveDirPath(binaryPath, relativeFlag)
		if err != nil {
			// Skip if we can't resolve path.
			continue
		}

		// Deduplicate PATH entries.
		if _, exists := seen[dirPath]; !exists {
			seen[dirPath] = struct{}{}
			pathEntries = append(pathEntries, dirPath)
		}

		toolPaths = append(toolPaths, ToolPath{
			Tool:    toolName,
			Version: version,
			Path:    dirPath,
		})
	}

	if len(pathEntries) == 0 {
		return nil, nil, fmt.Errorf("%w: no installed tools found from tool-versions file", ErrToolNotFound)
	}

	// Sort for consistent output.
	sort.Strings(pathEntries)
	sort.Slice(toolPaths, func(i, j int) bool {
		return toolPaths[i].Tool < toolPaths[j].Tool
	})

	return pathEntries, toolPaths, nil
}

// resolveDirPath resolves the directory path for a binary, either relative or absolute.
func resolveDirPath(binaryPath string, relativeFlag bool) (string, error) {
	if relativeFlag {
		// Use relative path.
		return filepath.Dir(binaryPath), nil
	}

	// Convert to absolute path (default behavior).
	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(absBinaryPath), nil
}

// getCurrentPath gets the current PATH environment variable with fallback.
func getCurrentPath() string {
	// Use viper which checks environment variables automatically.
	currentPath := viper.GetString("PATH")
	if currentPath == "" {
		// Default PATH for Unix-like systems (Windows uses PATH from environment).
		currentPath = strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin"}, string(os.PathListSeparator))
	}
	return currentPath
}

// constructFinalPath constructs the final PATH by prepending tool paths to current PATH.
func constructFinalPath(pathEntries []string, currentPath string) string {
	return strings.Join(pathEntries, string(os.PathListSeparator)) + string(os.PathListSeparator) + currentPath
}

// emitPathOutput outputs the PATH in the requested format.
func emitPathOutput(toolPaths []ToolPath, finalPath string, exportFlag, jsonFlag bool) error {
	switch {
	case jsonFlag:
		return emitJSONPath(toolPaths, finalPath)
	case exportFlag:
		return data.Writef("export PATH=\"%s\"\n", finalPath)
	default:
		return data.Writeln(finalPath)
	}
}
