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
	err = errUtils.Build(errUtils.ErrPathNotInComponentDir).
		WithHintf("Path `%s` is not within any configured component directories\n\nConfigured component base paths:\n  - Terraform: `%s`\n  - Helmfile: `%s`",
			absPath, atmosConfig.Components.Terraform.BasePath, atmosConfig.Components.Helmfile.BasePath).
		WithHint("Run `atmos describe config` to see configured component base paths\nEnsure you're in a component directory under one of the base paths above").
		WithContext("path", absPath).
		WithContext("terraform_base", atmosConfig.Components.Terraform.BasePath).
		WithContext("helmfile_base", atmosConfig.Components.Helmfile.BasePath).
		WithExitCode(2).
		Err()
	return nil, err
}

// normalizePathForResolution converts a path to absolute, clean, and symlink-resolved form.
func normalizePathForResolution(path string) (string, error) {
	// Convert "." to current working directory.
	if path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			pathErr := errUtils.Build(errUtils.ErrPathResolutionFailed).
				WithHintf("Failed to determine current working directory: %s", err.Error()).
				WithHint("Verify you have read permissions in the current directory\nTry providing an absolute path instead of `.`").
				WithContext("original_error", err.Error()).
				WithExitCode(1).
				Err()
			return "", pathErr
		}
		path = cwd
	}

	// Make absolute if relative.
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			pathErr := errUtils.Build(errUtils.ErrPathResolutionFailed).
				WithHintf("Failed to resolve absolute path for: `%s`\n%s", path, err.Error()).
				WithHint("Verify the path is valid and accessible").
				WithContext("path", path).
				WithContext("original_error", err.Error()).
				WithExitCode(1).
				Err()
			return "", pathErr
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
		baseErr := errUtils.Build(errUtils.ErrPathIsComponentBase).
			WithHintf("Cannot resolve component: path points to component base directory\nPath: `%s`\nComponent base: `%s`", absPath, basePath).
			WithHintf("Navigate into a specific component directory\nExample: `cd %s/vpc && atmos %s plan . --stack <stack>`",
				filepath.Base(basePath), componentType).
			WithContext("path", absPath).
			WithContext("base_path", basePath).
			WithContext("component_type", componentType).
			WithExitCode(2).
			Err()
		return nil, baseErr
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
		baseErr := errUtils.Build(errUtils.ErrPathIsComponentBase).
			WithHintf("Cannot resolve component: path points to component base directory\nPath: `%s`\nComponent base: `%s`", absPath, basePath).
			WithHintf("Navigate into a specific component directory\nExample: `cd %s/vpc && atmos %s plan . --stack <stack>`",
				filepath.Base(basePath), componentType).
			WithContext("path", absPath).
			WithContext("base_path", basePath).
			WithContext("component_type", componentType).
			WithExitCode(2).
			Err()
		return nil, baseErr
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
