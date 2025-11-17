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

	// 2. Check if the path points to known configuration directories (stacks, workflows).
	// These should not be treated as component paths.
	if err := validatePathIsNotConfigDirectory(atmosConfig, absPath); err != nil {
		return nil, err
	}

	// 3. Try to match against each component type.
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
	packerBasePath := atmosConfig.Components.Packer.BasePath
	if packerBasePath == "" {
		packerBasePath = "components/packer"
	}

	err = errUtils.Build(errUtils.ErrPathNotInComponentDir).
		WithHintf("Path `%s` is not within any configured component directories\n\nConfigured component base paths:\n  - Terraform: `%s`\n  - Helmfile: `%s`\n  - Packer: `%s`",
			absPath, atmosConfig.Components.Terraform.BasePath, atmosConfig.Components.Helmfile.BasePath, packerBasePath).
		WithHint("Change to a component directory and use `.` or provide a path within one of the component directories above").
		WithContext("path", absPath).
		WithContext("terraform_base", atmosConfig.Components.Terraform.BasePath).
		WithContext("helmfile_base", atmosConfig.Components.Helmfile.BasePath).
		WithContext("packer_base", packerBasePath).
		WithExitCode(2).
		Err()
	return nil, err
}

// validatePathIsNotConfigDirectory checks if the path points to a known configuration directory.
// Returns an error if the path is within stacks or workflows directories.
func validatePathIsNotConfigDirectory(atmosConfig *schema.AtmosConfiguration, absPath string) error {
	// Get absolute paths for stack and workflow directories.
	stacksBasePath := resolveBasePath(atmosConfig.Stacks.BasePath)
	workflowsBasePath := resolveBasePath(atmosConfig.Workflows.BasePath)

	// Check if path is within or equals stacks directory.
	if stacksBasePath != "" {
		stacksBasePath = filepath.Clean(stacksBasePath)
		if absPath == stacksBasePath || strings.HasPrefix(absPath, stacksBasePath+string(filepath.Separator)) {
			return errUtils.Build(errUtils.ErrPathNotInComponentDir).
				WithHintf("Path points to the stacks configuration directory, not a component:  \n%s\n\nStacks directory:  \n%s",
					absPath, stacksBasePath).
				WithHint("Components are located in component directories (terraform, helmfile, packer)  \nChange to a component directory and use `.` or provide a path within a component directory").
				WithContext("path", absPath).
				WithContext("stacks_base", stacksBasePath).
				WithExitCode(2).
				Err()
		}
	}

	// Check if path is within or equals workflows directory.
	if workflowsBasePath != "" {
		workflowsBasePath = filepath.Clean(workflowsBasePath)
		if absPath == workflowsBasePath || strings.HasPrefix(absPath, workflowsBasePath+string(filepath.Separator)) {
			return errUtils.Build(errUtils.ErrPathNotInComponentDir).
				WithHintf("Path points to the workflows directory, not a component:  \n%s\n\nWorkflows directory:  \n%s",
					absPath, workflowsBasePath).
				WithHint("Components are located in component directories (terraform, helmfile, packer)  \nChange to a component directory and use `.` or provide a path within a component directory").
				WithContext("path", absPath).
				WithContext("workflows_base", workflowsBasePath).
				WithExitCode(2).
				Err()
		}
	}

	return nil
}

// resolveBasePath converts a relative base path to absolute.
func resolveBasePath(basePath string) string {
	if basePath == "" || filepath.IsAbs(basePath) {
		return basePath
	}
	absPath, _ := filepath.Abs(basePath)
	return absPath
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

// ResolveAndCleanBasePath resolves a base path to absolute form and resolves symlinks.
func resolveAndCleanBasePath(basePath string) (string, error) {
	// Ensure base path is absolute.
	if !filepath.IsAbs(basePath) {
		basePathAbs, err := filepath.Abs(basePath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve base path: %w", err)
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
	} else {
		basePath = basePathResolved
	}

	return basePath, nil
}

// buildComponentBaseError creates a detailed error for when a path points to the component base directory.
func buildComponentBaseError(absPath, basePath, componentType string) error {
	return errUtils.Build(errUtils.ErrPathIsComponentBase).
		WithHintf("Cannot resolve component: path points to component base directory\nPath: `%s`\nComponent base: `%s`", absPath, basePath).
		WithHintf("Navigate into a specific component directory\nExample: `cd %s/vpc && atmos %s plan . --stack <stack>`",
			filepath.Base(basePath), componentType).
		WithContext("path", absPath).
		WithContext("base_path", basePath).
		WithContext("component_type", componentType).
		WithExitCode(2).
		Err()
}

func tryExtractComponentType(
	atmosConfig *schema.AtmosConfiguration,
	absPath string,
	componentType string,
) (*ComponentInfo, error) {
	// Get and resolve the base path for this component type.
	basePath, err := getResolvedBasePath(atmosConfig, componentType)
	if err != nil {
		return nil, err
	}

	// Compute relative path and validate it's within base path.
	relPath, err := validatePathWithinBase(absPath, basePath, componentType)
	if err != nil {
		return nil, err
	}

	// Parse the relative path into component parts.
	parts := parseRelativePathParts(relPath)
	if len(parts) == 0 {
		return nil, buildComponentBaseError(absPath, basePath, componentType)
	}

	// Build component info from parsed parts.
	return buildComponentInfo(componentType, parts), nil
}

// getResolvedBasePath gets the base path for a component type and resolves it to absolute.
func getResolvedBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, error) {
	basePath, _, err := getBasePathForComponentType(atmosConfig, componentType)
	if err != nil {
		return "", err
	}

	return resolveAndCleanBasePath(basePath)
}

// validatePathWithinBase validates that absPath is within basePath and returns the relative path.
func validatePathWithinBase(absPath, basePath, componentType string) (string, error) {
	relPath, err := filepath.Rel(basePath, absPath)
	if err != nil {
		return "", fmt.Errorf("path is not relative to %s base path: %w", componentType, err)
	}

	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("%w: %s", errUtils.ErrPathNotWithinComponentBase, componentType)
	}

	if relPath == "." {
		return "", buildComponentBaseError(absPath, basePath, componentType)
	}

	return relPath, nil
}

// parseRelativePathParts splits a relative path and filters out empty parts.
func parseRelativePathParts(relPath string) []string {
	parts := strings.Split(relPath, string(filepath.Separator))
	var nonEmptyParts []string
	for _, part := range parts {
		if part != "" {
			nonEmptyParts = append(nonEmptyParts, part)
		}
	}
	return nonEmptyParts
}

// buildComponentInfo constructs ComponentInfo from path parts.
func buildComponentInfo(componentType string, parts []string) *ComponentInfo {
	var folderPrefix, componentName, fullComponent string

	if len(parts) == 1 {
		// No folder prefix - component is directly in base path.
		componentName = parts[0]
		fullComponent = componentName
	} else {
		// Has folder prefix - everything except last part is folder prefix.
		folderPrefix = filepath.Join(parts[:len(parts)-1]...)
		componentName = parts[len(parts)-1]
		fullComponent = filepath.Join(folderPrefix, componentName)
	}

	// Convert Windows backslashes to forward slashes for component names.
	fullComponent = filepath.ToSlash(fullComponent)
	if folderPrefix != "" {
		folderPrefix = filepath.ToSlash(folderPrefix)
	}

	return &ComponentInfo{
		ComponentType: componentType,
		FolderPrefix:  folderPrefix,
		ComponentName: componentName,
		FullComponent: fullComponent,
	}
}
