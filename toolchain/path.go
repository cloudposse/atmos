package toolchain

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EmitPath outputs the PATH entries for installed toolchain binaries.
// The exportFlag outputs in shell export format, jsonFlag outputs JSON,
// and relativeFlag uses relative paths instead of absolute.
func EmitPath(exportFlag, jsonFlag, relativeFlag bool) error {
	defer perf.Track(nil, "toolchain.EmitPath")()

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

	// Build PATH entries for each tool.
	pathEntries, toolPaths, err := buildPathEntries(toolVersions, installer, relativeFlag)
	if err != nil {
		return err
	}

	// Get current PATH and construct final PATH.
	currentPath := getCurrentPath()
	finalPath := constructFinalPath(pathEntries, currentPath)

	// Output based on flags.
	return emitPathOutput(toolPaths, finalPath, exportFlag, jsonFlag)
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
