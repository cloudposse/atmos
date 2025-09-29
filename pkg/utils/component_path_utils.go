package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Static errors for component path resolution.
var (
	ErrUnknownComponentType = errors.New("unknown component type")
)

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
	// componentFolderPrefix might be empty, component might have subdirectories.
	pathParts := []string{basePath}
	if componentFolderPrefix != "" {
		pathParts = append(pathParts, componentFolderPrefix)
	}
	if component != "" {
		pathParts = append(pathParts, component)
	}

	componentPath := filepath.Join(pathParts...)

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
