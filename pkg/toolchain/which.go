package toolchain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

const versionSplit = "@"

// WhichExec prints the absolute path to the resolved tool binary for toolName (optionally with @version).
// Returns an error if the tool is not configured or not installed.
func WhichExec(toolName string) error {
	defer perf.Track(nil, "toolchain.WhichExec")()

	binaryPath, err := findBinaryPath(toolName)
	if err != nil {
		return err
	}
	return data.Writeln(binaryPath)
}

func findBinaryPath(toolNameFull string) (string, error) {
	// Extract tool name and version from input
	toolName := toolNameFull
	var version string
	if strings.Contains(toolName, versionSplit) {
		parts := strings.Split(toolNameFull, versionSplit)
		toolName = parts[0]
		version = parts[1]
	}

	// Look up the tool using the resolver, so aliases (e.g. "helm") match
	// entries stored under their canonical owner/repo key ("helm/helm") and
	// vice versa. The write side already canonicalizes for dedup — this keeps
	// the read side symmetric.
	installer := NewInstaller()
	resolvedKey := toolName
	if version == "" {
		toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
		if err != nil {
			return "", fmt.Errorf("failed to load .tool-versions file: %w", err)
		}
		key, defaultVersion, found := LookupToolVersion(toolName, toolVersions, installer.GetResolver())
		if !found {
			return "", fmt.Errorf("%w: tool '%s' not configured in .tool-versions", ErrToolNotFound, toolName)
		}
		// The first declared version is the default; explicit versions need no manifest.
		resolvedKey, version = key, defaultVersion
	}

	// Derive owner/repo from the resolved key, not the user input, so a raw
	// alias lookup finds the correct install path.
	owner, repo, err := installer.ParseToolSpec(resolvedKey)
	if err != nil {
		return "", fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
	}

	binaryPath, err := installer.FindBinaryPath(owner, repo, version)
	if errors.Is(err, ErrToolNotFound) {
		return "", fmt.Errorf("%w: tool '%s' is configured but not installed", err, toolName)
	}
	return binaryPath, err
}
