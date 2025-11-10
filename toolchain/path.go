package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EmitPath outputs the PATH entries for installed toolchain binaries.
// The exportFlag outputs in shell export format, jsonFlag outputs JSON,
// and relativeFlag uses relative paths instead of absolute.
func EmitPath(exportFlag, jsonFlag, relativeFlag bool) error {
	defer perf.Track(nil, "toolchain.EmitPath")()

	installer := NewInstaller()

	// Read tool-versions file
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: no tools configured in tool-versions file", ErrToolNotFound)
		}
		return fmt.Errorf("%w: reading %s: %v", ErrFileOperation, GetToolVersionsFilePath(), err)
	}

	if len(toolVersions.Tools) == 0 {
		return fmt.Errorf("%w: no tools installed from .tool-versions file", ErrToolNotFound)
	}

	// Build PATH entries for each tool
	var pathEntries []string
	var toolPaths []ToolPath
	seen := make(map[string]struct{}) // Track seen paths to avoid duplicates

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}
		version := versions[0] // default version
		// Resolve tool name to owner/repo using parseToolSpec for consistency
		owner, repo, err := installer.parseToolSpec(toolName)
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

		// Deduplicate PATH entries
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
		return fmt.Errorf("%w: no installed tools found from tool-versions file", ErrToolNotFound)
	}

	// Sort for consistent output
	sort.Strings(pathEntries)
	sort.Slice(toolPaths, func(i, j int) bool {
		return toolPaths[i].Tool < toolPaths[j].Tool
	})

	// Get current PATH
	// Use viper which checks environment variables automatically.
	currentPath := viper.GetString("PATH")
	if currentPath == "" {
		// Default PATH for Unix-like systems (Windows uses PATH from environment).
		currentPath = strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin"}, string(os.PathListSeparator))
	}

	// Construct final PATH using platform-specific separator.
	finalPath := strings.Join(pathEntries, string(os.PathListSeparator)) + string(os.PathListSeparator) + currentPath

	// Output based on flags
	switch {
	case jsonFlag:
		return emitJSONPath(toolPaths, finalPath)
	case exportFlag:
		return data.Writef("export PATH=\"%s\"\n", finalPath)
	default:
		return data.Writeln(finalPath)
	}
}

// ToolPath represents a tool with its version and path.
type ToolPath struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

func emitJSONPath(toolPaths []ToolPath, finalPath string) error {
	output := struct {
		Tools     []ToolPath `json:"tools"`
		FinalPath string     `json:"final_path"`
		Count     int        `json:"count"`
	}{
		Tools:     toolPaths,
		FinalPath: finalPath,
		Count:     len(toolPaths),
	}
	return data.WriteJSON(output)
}
