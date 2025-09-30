package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Static errors for component path resolution.
var (
	ErrUnknownComponentType = errors.New("unknown component type")
)

// Constants for path handling.
const (
	windowsPathSeparator = `\`
	uncPrefix            = `\\`
)

// isUNCPath checks if a path is a UNC path (\\server\share format).
func isUNCPath(path string) bool {
	return strings.HasPrefix(path, uncPrefix) && len(strings.Split(path, windowsPathSeparator)) >= 4
}

// ensureAbsolutePath converts a path to absolute while preserving UNC paths.
func ensureAbsolutePath(path string) (string, error) {
	// UNC paths are already absolute, don't use filepath.Abs() which can corrupt them.
	if isUNCPath(path) {
		return path, nil
	}

	// For regular paths, use filepath.Abs().
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path: %w", err)
		}
		return absPath, nil
	}

	return path, nil
}

// joinUNCPath joins a UNC base path with additional components using Windows separators.
func joinUNCPath(basePath, component string) string {
	if component == "" {
		return basePath
	}

	result := basePath
	if !strings.HasSuffix(result, windowsPathSeparator) {
		result += windowsPathSeparator
	}

	// Convert forward slashes to backslashes for UNC paths.
	component = strings.ReplaceAll(component, "/", windowsPathSeparator)
	result += component

	return result
}

// cleanUNCPath cleans a UNC path while preserving the UNC prefix.
func cleanUNCPath(path string) string {
	if !strings.HasPrefix(path, uncPrefix) {
		return path
	}

	cleanPath := path
	// Remove any sequences of 3 or more consecutive backslashes, keeping only single backslashes.
	// We need to be careful to preserve the UNC prefix (\\).
	for {
		// Look for any sequence of 3+ backslashes.
		tripleOrMore := windowsPathSeparator + windowsPathSeparator + windowsPathSeparator
		if !strings.Contains(cleanPath, tripleOrMore) {
			break
		}
		// Replace sequences of 3+ backslashes with single backslash.
		cleanPath = strings.ReplaceAll(cleanPath, tripleOrMore, windowsPathSeparator)
	}

	// Handle double backslashes that aren't the UNC prefix.
	// Split at UNC prefix, clean the rest, then rejoin.
	if len(cleanPath) > 2 {
		suffix := cleanPath[2:] // Everything after the UNC prefix
		// Replace any remaining double backslashes in the suffix with single backslash.
		for strings.Contains(suffix, windowsPathSeparator+windowsPathSeparator) {
			suffix = strings.ReplaceAll(suffix, windowsPathSeparator+windowsPathSeparator, windowsPathSeparator)
		}
		cleanPath = uncPrefix + suffix
	}

	return cleanPath
}

// getBasePathForComponentType returns the base path for a specific component type.
func getBasePathForComponentType(atmosConfig *schema.AtmosConfiguration, componentType string) (string, string, error) {
	var basePath string
	var envVarName string
	var resolvedPath string
	var configBasePath string

	switch componentType {
	case "terraform":
		envVarName = "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"
		resolvedPath = atmosConfig.TerraformDirAbsolutePath
		configBasePath = atmosConfig.Components.Terraform.BasePath
	case "helmfile":
		envVarName = "ATMOS_COMPONENTS_HELMFILE_BASE_PATH"
		resolvedPath = atmosConfig.HelmfileDirAbsolutePath
		configBasePath = atmosConfig.Components.Helmfile.BasePath
	case "packer":
		envVarName = "ATMOS_COMPONENTS_PACKER_BASE_PATH"
		resolvedPath = atmosConfig.PackerDirAbsolutePath
		configBasePath = atmosConfig.Components.Packer.BasePath
	default:
		return "", "", fmt.Errorf("%w: %s", ErrUnknownComponentType, componentType)
	}

	// Check for env var override first - this completely overrides any config.
	// Note: We use os.Getenv here because these are test-specific environment variables
	// that are set by the sandbox for test isolation, not user configuration.
	if envPath := os.Getenv(envVarName); envPath != "" { //nolint:forbidigo
		log.Debug("Using component base path from environment variable",
			"var", envVarName,
			"path", envPath)
		basePath = envPath
	} else if resolvedPath != "" {
		// Use pre-resolved absolute path.
		basePath = resolvedPath
	} else {
		// Construct from configured paths (could be anything - opentofu/, tf/, etc.).
		// Handle UNC paths specially to preserve their format.
		if isUNCPath(atmosConfig.BasePath) {
			basePath = joinUNCPath(atmosConfig.BasePath, configBasePath)
		} else {
			basePath = filepath.Join(atmosConfig.BasePath, configBasePath)
		}
	}

	return basePath, envVarName, nil
}

// GetComponentPath returns the absolute path to a component, respecting all configuration and overrides.
// Priority order:
// 1. Environment variables (if set, these completely override config)
// 2. Already-resolved absolute paths in atmosConfig
// 3. Constructed from BasePath + configured component paths
//
// This function makes NO assumptions about directory names - it only uses what's configured.
func GetComponentPath(atmosConfig *schema.AtmosConfiguration, componentType string, componentFolderPrefix string, component string) (string, error) {
	basePath, envVarName, err := getBasePathForComponentType(atmosConfig, componentType)
	if err != nil {
		return "", err
	}

	// Ensure base path is absolute while preserving UNC paths.
	absPath, err := ensureAbsolutePath(basePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %s base path '%s': %w",
			componentType, basePath, err)
	}
	basePath = absPath

	// Build the full component path.
	var componentPath string
	if isUNCPath(basePath) {
		// For UNC paths, use helper function to preserve UNC format.
		componentPath = joinUNCPath(basePath, componentFolderPrefix)
		componentPath = joinUNCPath(componentPath, component)
	} else {
		// For regular paths, use filepath.Join().
		pathParts := []string{basePath}
		if componentFolderPrefix != "" {
			pathParts = append(pathParts, componentFolderPrefix)
		}
		if component != "" {
			pathParts = append(pathParts, component)
		}
		componentPath = filepath.Join(pathParts...)
	}

	// Clean the path to handle any redundant separators or relative components.
	var cleanPath string
	if isUNCPath(componentPath) {
		cleanPath = cleanUNCPath(componentPath)
	} else {
		cleanPath = filepath.Clean(componentPath)
	}

	log.Debug("Resolved component path",
		"type", componentType,
		"component", component,
		"resolved_path", cleanPath,
		"base_path", basePath,
		"env_override", os.Getenv(envVarName) != "", //nolint:forbidigo
	)

	return cleanPath, nil
}

// GetComponentBasePath returns just the base path for a component type.
// Useful when you need the base directory without a specific component.
func GetComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, error) {
	return GetComponentPath(atmosConfig, componentType, "", "")
}
