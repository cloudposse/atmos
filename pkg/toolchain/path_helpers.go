package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ToolPath represents a tool with its version and path.
type ToolPath struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

// binaryPathsLocator is an optional extension of InstallLocator that returns
// every installed entrypoint path for a tool version. A onedir (multi-file)
// package exposes multiple commands that may live in different directories, so
// each of their directories must be added to PATH. The real *installer.Installer
// implements it; locators that only implement InstallLocator (e.g. test mocks)
// keep the primary-only behavior.
type binaryPathsLocator interface {
	GetBinaryPaths(owner, repo, version string) []string
}

// buildPathEntries constructs PATH entries from tool versions.
// This is a backward-compatible wrapper around buildPathEntriesWithLocator.
func buildPathEntries(toolVersions *ToolVersions, installer *Installer, relativeFlag bool) ([]string, []ToolPath, error) {
	return buildPathEntriesWithLocator(toolVersions, installer, relativeFlag)
}

// entrypointBinaryPaths returns every installed entrypoint path for a tool. For
// a onedir package it returns all manifest entrypoints (via the optional
// binaryPathsLocator); otherwise it falls back to the single primary path
// already resolved by FindBinaryPath (which handles legacy layouts and the
// "latest" keyword).
func entrypointBinaryPaths(locator InstallLocator, owner, repo, version, primary string) []string {
	if bpl, ok := locator.(binaryPathsLocator); ok {
		if paths := bpl.GetBinaryPaths(owner, repo, version); len(paths) > 0 {
			return paths
		}
	}
	return []string{primary}
}

// entrypointDirs resolves the (relative or absolute) directory of each given
// binary path, skipping any that cannot be resolved.
func entrypointDirs(binaryPaths []string, relativeFlag bool) []string {
	dirs := make([]string, 0, len(binaryPaths))
	for _, bp := range binaryPaths {
		dir, err := resolveDirPath(bp, relativeFlag)
		if err != nil {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

// EntrypointDirsForVersion returns the directories that must be on PATH to expose
// an installed tool version's entrypoints, using the same onedir-aware resolution
// as `atmos toolchain env`. For a onedir (multi-file) package it returns each
// directory holding a resolved manifest entrypoint (e.g. the nested .pkg/.../bin);
// for a flat install it returns the single version directory. It returns one entry
// per resolved entrypoint (callers deduplicate), and nil when the tool version is
// not installed, letting the caller fall back to a bare version-dir guess for
// backward compatibility.
//
// When relativeFlag is true the directories are relative to the locator's bin
// dir; otherwise they are absolute (the common case for building a subprocess
// PATH).
func EntrypointDirsForVersion(locator InstallLocator, owner, repo, version string, relativeFlag bool) []string {
	defer perf.Track(nil, "toolchain.EntrypointDirsForVersion")()

	binaryPath, err := locator.FindBinaryPath(owner, repo, version)
	if err != nil {
		// Tool not installed (or unusual layout): let the caller decide the fallback.
		return nil
	}
	return entrypointDirs(entrypointBinaryPaths(locator, owner, repo, version, binaryPath), relativeFlag)
}

// appendUniqueDirs appends dirs not already in seen to pathEntries.
func appendUniqueDirs(pathEntries []string, seen map[string]struct{}, dirs []string) []string {
	for _, dir := range dirs {
		if _, exists := seen[dir]; !exists {
			seen[dir] = struct{}{}
			pathEntries = append(pathEntries, dir)
		}
	}
	return pathEntries
}

// buildPathEntriesWithLocator constructs PATH entries from tool versions using an InstallLocator.
// This function accepts an interface to allow mocking in tests.
func buildPathEntriesWithLocator(toolVersions *ToolVersions, locator InstallLocator, relativeFlag bool) ([]string, []ToolPath, error) {
	var pathEntries []string
	var toolPaths []ToolPath
	seen := make(map[string]struct{}) // Track seen paths to avoid duplicates.

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}

		version := versions[0] // Default version.

		// Resolve tool name to owner/repo using ParseToolSpec for consistency.
		owner, repo, err := locator.ParseToolSpec(toolName)
		if err != nil {
			// Skip tools that can't be resolved.
			continue
		}

		// Find the actual binary path (handles path inconsistencies).
		binaryPath, err := locator.FindBinaryPath(owner, repo, version)
		if err != nil {
			// Tool not installed, skip it.
			continue
		}

		dirPath, err := resolveDirPath(binaryPath, relativeFlag)
		if err != nil {
			// Skip if we can't resolve path.
			continue
		}

		// Add every entrypoint directory (a onedir package exposes multiple
		// commands that may live in different directories), deduplicated.
		allPaths := entrypointBinaryPaths(locator, owner, repo, version, binaryPath)
		pathEntries = appendUniqueDirs(pathEntries, seen, entrypointDirs(allPaths, relativeFlag))

		// Record one entry per configured tool, keyed to its primary directory.
		toolPaths = append(toolPaths, ToolPath{Tool: toolName, Version: version, Path: dirPath})
	}

	if len(pathEntries) == 0 {
		return nil, nil, errUtils.Build(ErrToolNotFound).
			WithExplanation("no installed tools found from tool-versions file").
			WithHint("Run 'atmos toolchain add <tool@version>' to add and install tools").
			Err()
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
	//nolint:forbidigo // PATH is a system env var, not an Atmos config
	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		// Fallback for edge cases where PATH isn't set (rare).
		// Use OS-specific default paths.
		if runtime.GOOS == "windows" {
			//nolint:forbidigo // SystemRoot/WINDIR are Windows system env vars, not Atmos config
			systemRoot := os.Getenv("SystemRoot")
			if systemRoot == "" {
				//nolint:forbidigo // SystemRoot/WINDIR are Windows system env vars, not Atmos config
				systemRoot = os.Getenv("WINDIR")
			}
			if systemRoot != "" {
				currentPath = strings.Join([]string{
					filepath.Join(systemRoot, "System32"),
					filepath.Join(systemRoot, "System32", "Wbem"),
					filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0"),
				}, string(os.PathListSeparator))
			}
		} else {
			currentPath = strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin"}, string(os.PathListSeparator))
		}
	}
	return currentPath
}

// constructFinalPath constructs the final PATH by prepending tool paths to current PATH.
func constructFinalPath(pathEntries []string, currentPath string) string {
	return strings.Join(pathEntries, string(os.PathListSeparator)) + string(os.PathListSeparator) + currentPath
}
