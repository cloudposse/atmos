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

// EmitEnv outputs the PATH entries for installed toolchain binaries in shell-specific format.
// The format parameter specifies the output format (bash, fish, powershell, json, dotenv),
// and relativeFlag uses relative paths instead of absolute.
func EmitEnv(format string, relativeFlag bool) error {
	defer perf.Track(nil, "toolchain.EmitEnv")()

	installer := NewInstaller()

	// Read tool-versions file.
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

	// Build PATH entries for each tool.
	var pathEntries []string
	var toolPaths []ToolPath
	seen := make(map[string]struct{}) // Track seen paths to avoid duplicates.

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}
		version := versions[0] // default version.
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

		var dirPath string
		if relativeFlag {
			// Use relative path.
			dirPath = filepath.Dir(binaryPath)
		} else {
			// Convert to absolute path (default behavior).
			absBinaryPath, err := filepath.Abs(binaryPath)
			if err != nil {
				// Skip if we can't get absolute path.
				continue
			}
			dirPath = filepath.Dir(absBinaryPath)
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
		return fmt.Errorf("%w: no installed tools found from tool-versions file", ErrToolNotFound)
	}

	// Sort for consistent output.
	sort.Strings(pathEntries)
	sort.Slice(toolPaths, func(i, j int) bool {
		return toolPaths[i].Tool < toolPaths[j].Tool
	})

	// Get current PATH.
	// Use viper which checks environment variables automatically.
	currentPath := viper.GetString("PATH")
	if currentPath == "" {
		// Default PATH for Unix-like systems (Windows uses PATH from environment).
		currentPath = strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin"}, string(os.PathListSeparator))
	}

	// Construct final PATH using platform-specific separator.
	finalPath := strings.Join(pathEntries, string(os.PathListSeparator)) + string(os.PathListSeparator) + currentPath

	// Output based on format.
	switch format {
	case "json":
		return emitJSONPath(toolPaths, finalPath)
	case "bash":
		return emitBashEnv(finalPath)
	case "dotenv":
		return emitDotenvEnv(finalPath)
	case "fish":
		return emitFishEnv(finalPath)
	case "powershell":
		return emitPowershellEnv(finalPath)
	default:
		return emitBashEnv(finalPath)
	}
}

// emitBashEnv outputs PATH as bash/zsh export statement.
func emitBashEnv(finalPath string) error {
	// Escape single quotes for safe single-quoted shell literals: ' -> '\''
	safe := strings.ReplaceAll(finalPath, "'", "'\\''")
	return data.Writef("export PATH='%s'\n", safe)
}

// emitDotenvEnv outputs PATH in .env format.
func emitDotenvEnv(finalPath string) error {
	// Use the same safe single-quoted escaping as bash output.
	safe := strings.ReplaceAll(finalPath, "'", "'\\''")
	return data.Writef("PATH='%s'\n", safe)
}

// emitFishEnv outputs PATH as fish shell set command.
func emitFishEnv(finalPath string) error {
	// Fish uses space-separated paths, not colon-separated.
	// Convert platform-specific separator to spaces.
	paths := strings.Split(finalPath, string(os.PathListSeparator))
	// Escape single quotes for fish: ' -> \'
	var escapedPaths []string
	for _, p := range paths {
		escaped := strings.ReplaceAll(p, "'", "\\'")
		escapedPaths = append(escapedPaths, fmt.Sprintf("'%s'", escaped))
	}
	return data.Writef("set -gx PATH %s\n", strings.Join(escapedPaths, " "))
}

// emitPowershellEnv outputs PATH as PowerShell $env: assignment.
func emitPowershellEnv(finalPath string) error {
	// PowerShell uses semicolon as path separator on Windows, but we use platform-specific.
	// Escape double quotes and dollar signs for PowerShell strings.
	safe := strings.ReplaceAll(finalPath, "\"", "`\"")
	safe = strings.ReplaceAll(safe, "$", "`$")
	return data.Writef("$env:PATH = \"%s\"\n", safe)
}
