package toolchain

import (
	"fmt"
	"os"
	"strings"

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
		return fmt.Errorf("%w: reading %s: %w", ErrFileOperation, GetToolVersionsFilePath(), err)
	}

	if len(toolVersions.Tools) == 0 {
		return fmt.Errorf("%w: no tools installed from .tool-versions file", ErrToolNotFound)
	}

	// Build PATH entries for each tool (reuse helper from path.go).
	pathEntries, toolPaths, err := buildPathEntries(toolVersions, installer, relativeFlag)
	if err != nil {
		return err
	}

	// Get current PATH and construct final PATH.
	currentPath := getCurrentPath()
	finalPath := constructFinalPath(pathEntries, currentPath)

	// Output based on format.
	return emitEnvOutput(toolPaths, finalPath, format)
}

// emitEnvOutput outputs environment variables in the requested format.
func emitEnvOutput(toolPaths []ToolPath, finalPath, format string) error {
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
