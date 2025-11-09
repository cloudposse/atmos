package toolchain

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

const versionSplit = "@"

func init() {
	// No flags needed for which command
}

func WhichExec(toolName string) error {
	defer perf.Track(nil, "toolchain.WhichExec")()

	binaryPath, err := findBinaryPath(toolName)
	if err != nil {
		return err
	}
	return data.Writeln(binaryPath)
}

func findBinaryPath(toolNameFull string) (string, error) {
	// Check if the tool is configured in .tool-versions
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		return "", fmt.Errorf("failed to load .tool-versions file: %w", err)
	}

	// Extract tool name and version from input
	toolName := toolNameFull
	var version string
	if strings.Contains(toolName, versionSplit) {
		parts := strings.Split(toolNameFull, versionSplit)
		toolName = parts[0]
		version = parts[1]
	}

	versions, exists := toolVersions.Tools[toolName]
	if !exists || len(versions) == 0 {
		return "", fmt.Errorf("%w: tool '%s' not configured in .tool-versions", ErrToolNotFound, toolName)
	}

	// Use the most recent version if not specified
	if version == "" {
		version = versions[len(versions)-1]
	}

	// Now that we know the tool is configured, use the installer to resolve the canonical name
	// and get the binary path
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
	}

	binaryPath := installer.getBinaryPath(owner, repo, version)

	// Check if the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return "", fmt.Errorf("%w: tool '%s' is configured but not installed", ErrToolNotFound, toolName)
	}

	return binaryPath, nil
}
