package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentInfo represents extracted component information from a filesystem path.
type ComponentInfo struct {
	ComponentType string // "terraform", "helmfile", "packer"
	FolderPrefix  string // "vpc", "networking/vpc", etc. (empty if component is at base)
	ComponentName string // "security-group", "vpc", etc.
	FullComponent string // Full component path as it appears in stack config (folder_prefix/component or just component)
}

// ExtractComponentInfoFromPath extracts component information from an absolute filesystem path.
// Returns error if path is not within configured component directories.
//
// Examples:
//   - /project/components/terraform/vpc/security-group → {terraform, vpc, security-group, vpc/security-group}
//   - /project/components/terraform/vpc → {terraform, "", vpc, vpc}
//   - /project/components/helmfile/app → {helmfile, "", app, app}
func ExtractComponentInfoFromPath(
	atmosConfig *schema.AtmosConfiguration,
	path string,
) (*ComponentInfo, error) {
	defer perf.Track(atmosConfig, "utils.ExtractComponentInfoFromPath")()

	// 1. Normalize path (absolute, clean, resolve symlinks).
	absPath, err := normalizePathForResolution(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	log.Debug("Extracting component info from path", "original", path, "normalized", absPath)

	// 2. Try to match against each component type.
	componentTypes := []string{"terraform", "helmfile", "packer"}
	for _, componentType := range componentTypes {
		info, err := tryExtractComponentType(atmosConfig, absPath, componentType)
		if err == nil {
			log.Debug("Successfully extracted component info",
				"type", info.ComponentType,
				"folder_prefix", info.FolderPrefix,
				"component", info.ComponentName,
				"full", info.FullComponent,
			)
			return info, nil
		}
		log.Trace("Path does not match component type", "type", componentType, "error", err)
	}

	// None of the component types matched.
	return nil, fmt.Errorf("%w: %s", errUtils.ErrPathNotInComponentDir, absPath)
}

// normalizePathForResolution converts a path to absolute, clean, and symlink-resolved form.
func normalizePathForResolution(path string) (string, error) {
	// Convert "." to current working directory.
	if path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		path = cwd
	}

	// Make absolute if relative.
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path: %w", err)
		}
		path = absPath
	}

	// Resolve symlinks.
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If symlink resolution fails (e.g., path doesn't exist), continue with original path.
		// This allows for better error messages later.
		log.Trace("Could not resolve symlinks", "path", path, "error", err)
		resolvedPath = path
	}

	// Clean the path (remove . and .., normalize separators).
	cleanPath := filepath.Clean(resolvedPath)

	return cleanPath, nil
}

// tryExtractComponentType attempts to extract component info for a specific component type.
func tryExtractComponentType(
	atmosConfig *schema.AtmosConfiguration,
	absPath string,
	componentType string,
) (*ComponentInfo, error) {
	// Get the base path for this component type.
	basePath, _, err := getBasePathForComponentType(atmosConfig, componentType)
	if err != nil {
		return nil, err
	}

	// Ensure base path is absolute.
	if !filepath.IsAbs(basePath) {
		basePathAbs, err := filepath.Abs(basePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve base path: %w", err)
		}
		basePath = basePathAbs
	}

	// Clean base path.
	basePath = filepath.Clean(basePath)

	// Resolve symlinks in base path (important on macOS where /var -> /private/var).
	basePathResolved, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		// If symlink resolution fails, continue with original path.
		log.Trace("Could not resolve symlinks in base path", "path", basePath, "error", err)
		basePathResolved = basePath
	} else {
		basePath = basePathResolved
	}

	// Check if absPath is within basePath.
	relPath, err := filepath.Rel(basePath, absPath)
	if err != nil {
		return nil, fmt.Errorf("path is not relative to %s base path: %w", componentType, err)
	}

	// If relPath starts with "..", it's not within basePath.
	if strings.HasPrefix(relPath, "..") {
		return nil, fmt.Errorf("path is not within %s base path", componentType)
	}

	// If relPath is ".", the path IS the base path (not allowed).
	if relPath == "." {
		return nil, fmt.Errorf("%w", errUtils.ErrPathIsComponentBase)
	}

	// Split the relative path into parts.
	parts := strings.Split(relPath, string(filepath.Separator))

	// Filter out empty parts (can happen on Windows or with trailing slashes).
	var nonEmptyParts []string
	for _, part := range parts {
		if part != "" {
			nonEmptyParts = append(nonEmptyParts, part)
		}
	}

	if len(nonEmptyParts) == 0 {
		return nil, fmt.Errorf("%w", errUtils.ErrPathIsComponentBase)
	}

	// Determine folder prefix and component name.
	var folderPrefix string
	var componentName string
	var fullComponent string

	if len(nonEmptyParts) == 1 {
		// No folder prefix - component is directly in base path.
		// Example: components/terraform/vpc → component="vpc"
		componentName = nonEmptyParts[0]
		fullComponent = componentName
	} else {
		// Has folder prefix - everything except last part is folder prefix.
		// Example: components/terraform/vpc/security-group → folder="vpc", component="security-group"
		folderPrefix = filepath.Join(nonEmptyParts[:len(nonEmptyParts)-1]...)
		componentName = nonEmptyParts[len(nonEmptyParts)-1]
		fullComponent = filepath.Join(folderPrefix, componentName)
	}

	// Convert Windows backslashes to forward slashes for component names.
	// Component names in stack configs always use forward slashes.
	fullComponent = filepath.ToSlash(fullComponent)
	if folderPrefix != "" {
		folderPrefix = filepath.ToSlash(folderPrefix)
	}

	return &ComponentInfo{
		ComponentType: componentType,
		FolderPrefix:  folderPrefix,
		ComponentName: componentName,
		FullComponent: fullComponent,
	}, nil
}
