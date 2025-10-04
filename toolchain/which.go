package toolchain

import (
	"fmt"
	"os"
	"strings"
)

const versionSplit = "@"

func init() {
	// No flags needed for which command
}

func WhichExec(toolName string) error {
	binaryPath, err := findBinaryPath(toolName)
	if err != nil {
		return err
	}
	fmt.Println(binaryPath)
	return nil
}

func findBinaryPath(toolNameFull string) (string, error) {
	// Check if the tool is configured in .tool-versions
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		return "", fmt.Errorf("failed to load .tool-versions file: %w", err)
	}
	toolName := toolNameFull
	if strings.Contains(toolName, versionSplit) {
		toolName = strings.Split(toolName, versionSplit)[0]
	}

	versions, exists := toolVersions.Tools[toolName]
	if !exists || len(versions) == 0 {
		return "", fmt.Errorf("tool '%s' not configured in .tool-versions", toolName)
	}

	// Use the most recent version
	version := versions[len(versions)-1]
	if strings.Contains(toolNameFull, versionSplit) {
		version = strings.Split(toolNameFull, versionSplit)[1]
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
		return "", fmt.Errorf("tool '%s' is configured but not installed", toolName)
	}

	return binaryPath, nil
}
