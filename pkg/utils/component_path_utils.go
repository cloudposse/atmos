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

// cleanDuplicatedPath detects and removes path duplication patterns.
// For example: /path/to/base/.//path/to/base/components -> /path/to/base/components
// This only removes duplications when a significant path segment is duplicated,
// not just when a single directory name appears multiple times.
func cleanDuplicatedPath(path string) string {
	if path == "" {
		return ""
	}

	// First clean the path to normalize separators and remove ./ patterns
	cleanedPath := filepath.Clean(path)

	// Look for patterns like /base/path/.//base/path or /base/path/base/path
	// Split the path into parts
	parts := strings.Split(cleanedPath, string(filepath.Separator))

	// For absolute paths, the first part will be empty, skip it
	startIdx := 0
	if len(parts) > 0 && parts[0] == "" {
		startIdx = 1
	}

	// Only look for duplications of sequences that are at least 3 parts long
	// This avoids removing legitimate repeated single directory names
	minLength := 3
	if len(parts)-startIdx < minLength*2 {
		// Not enough parts to have a significant duplication
		return cleanedPath
	}

	// Look for consecutive duplicate sequences of significant length
	for length := minLength; length <= (len(parts)-startIdx)/2; length++ {
		// Check if a sequence of 'length' parts repeats immediately after itself
		for start := startIdx; start+length*2 <= len(parts); start++ {
			sequenceRepeats := true
			for j := 0; j < length; j++ {
				if parts[start+j] != parts[start+length+j] {
					sequenceRepeats = false
					break
				}
			}
			if sequenceRepeats {
				// Found a duplicate sequence, remove it
				newParts := append(parts[:start+length], parts[start+length*2:]...)
				cleanedPath = filepath.Join(newParts...)
				if filepath.IsAbs(path) && !filepath.IsAbs(cleanedPath) {
					cleanedPath = string(filepath.Separator) + cleanedPath
				}
				// Recursively clean in case there are more duplications
				return cleanDuplicatedPath(cleanedPath)
			}
		}
	}

	return cleanedPath
}

// buildComponentPath builds the component path handling absolute vs relative cases.
func buildComponentPath(basePath, componentFolderPrefix, component string) string {
	// Check if the component itself is an absolute path
	if component != "" && filepath.IsAbs(component) {
		// If component is absolute, use it as the base and only append folder prefix if needed
		if componentFolderPrefix != "" {
			return filepath.Join(component, componentFolderPrefix)
		}
		return component
	}

	// Build path step by step using JoinPath to handle absolute paths correctly
	result := basePath
	if componentFolderPrefix != "" {
		result = JoinPath(result, componentFolderPrefix)
	}
	if component != "" {
		result = JoinPath(result, component)
	}
	return result
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
		basePath = filepath.Join(atmosConfig.BasePath, configBasePath)
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

	// Clean up path duplication that might occur from incorrect configuration
	basePath = cleanDuplicatedPath(basePath)

	// Ensure base path is absolute.
	if !filepath.IsAbs(basePath) {
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path for %s base path '%s': %w",
				componentType, basePath, err)
		}
		basePath = absPath
	}

	// Build the full component path.
	componentPath := buildComponentPath(basePath, componentFolderPrefix, component)

	// Clean the path to handle any redundant separators or relative components.
	cleanPath := filepath.Clean(componentPath)

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
