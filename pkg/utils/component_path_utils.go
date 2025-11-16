package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
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

// hasSequenceRepeat checks if parts[start:start+length] equals parts[start+length:start+length*2].
func hasSequenceRepeat(parts []string, start, length int) bool {
	for j := 0; j < length; j++ {
		if parts[start+j] != parts[start+length+j] {
			return false
		}
	}
	return true
}

// removeDuplicateSequence removes a duplicate sequence from path parts.
func removeDuplicateSequence(parts []string, start, length int, originalPath string) string {
	// Extract the volume/UNC prefix from the original path
	volume := filepath.VolumeName(originalPath)

	newParts := make([]string, 0, len(parts)-length)
	newParts = append(newParts, parts[:start+length]...)
	newParts = append(newParts, parts[start+length*2:]...)
	cleanedPath := strings.Join(newParts, string(filepath.Separator))

	// Handle different volume/path scenarios
	return preserveVolume(cleanedPath, volume, originalPath)
}

// preserveVolume preserves UNC paths and volume names in the cleaned path.
func preserveVolume(cleanedPath, volume, originalPath string) string {
	if volume == "" {
		// No volume - just ensure Unix absolute paths stay absolute
		if filepath.IsAbs(originalPath) && !filepath.IsAbs(cleanedPath) {
			return string(filepath.Separator) + cleanedPath
		}
		return cleanedPath
	}

	// Check if it's a UNC path (starts with \\)
	if isUNCPath(volume) {
		return handleUNCPath(cleanedPath, volume)
	}

	// For regular volumes (like C:), ensure proper Windows drive-root style path
	if cleanedPath == "" {
		return volume + string(filepath.Separator)
	}
	if strings.HasPrefix(cleanedPath, string(filepath.Separator)) {
		return volume + cleanedPath
	}
	return volume + string(filepath.Separator) + cleanedPath
}

// isUNCPath checks if the volume is a UNC path.
func isUNCPath(volume string) bool {
	return strings.HasPrefix(volume, uncPrefix)
}

// handleUNCPath processes UNC paths to avoid duplication.
func handleUNCPath(cleanedPath, volume string) string {
	// Always reconstruct the path using the original volume to preserve UNC prefix
	volumeWithoutPrefix := strings.TrimPrefix(volume, uncPrefix)

	// Try to strip the full volume first
	remainder := strings.TrimPrefix(cleanedPath, volume)
	if remainder == cleanedPath {
		// If full volume stripping didn't work, try without UNC prefix
		remainder = strings.TrimPrefix(cleanedPath, volumeWithoutPrefix)
	}

	// Trim any leading path separators from the remainder
	remainder = strings.TrimLeft(remainder, windowsPathSeparator)

	// If no remainder, return just the volume
	if remainder == "" {
		return volume
	}

	// Reconstruct with original volume + separator + remainder
	return volume + windowsPathSeparator + remainder
}

// CleanDuplicatedPath removes duplicate path segments that sometimes occur due to
// symlink resolution or path joining issues.
// For example: /foo/bar/foo/bar/baz becomes /foo/bar/baz.
func CleanDuplicatedPath(path string) string {
	return cleanDuplicatedPath(path)
}

// cleanDuplicatedPath detects and removes path duplication patterns.
// For example: /path/to/base/.//path/to/base/components -> /path/to/base/components
// This only removes duplications when a significant path segment is duplicated,
// not just when a single directory name appears multiple times.
func cleanDuplicatedPath(path string) string {
	defer perf.Track(nil, "utils.cleanDuplicatedPath")()

	if path == "" {
		return ""
	}

	// First clean the path to normalize separators and remove ./ patterns
	cleanedPath := filepath.Clean(path)

	// Split the path into parts
	parts := strings.Split(cleanedPath, string(filepath.Separator))

	// For absolute paths, the first part will be empty, skip it
	startIdx := 0
	if len(parts) > 0 && parts[0] == "" {
		startIdx = 1
	}

	// Only look for duplications of sequences that are at least 2 parts long.
	// This catches patterns like /tests/fixtures/tests/fixtures (2-part duplication).
	minLength := 2
	if len(parts)-startIdx < minLength*2 {
		return cleanedPath
	}

	// Look for consecutive duplicate sequences of significant length
	for length := minLength; length <= (len(parts)-startIdx)/2; length++ {
		for start := startIdx; start+length*2 <= len(parts); start++ {
			if hasSequenceRepeat(parts, start, length) {
				// Found a duplicate sequence, remove it and recurse
				result := removeDuplicateSequence(parts, start, length, path)
				return cleanDuplicatedPath(result)
			}
		}
	}

	return cleanedPath
}

// buildComponentPath builds the component path handling absolute vs relative cases.
func buildComponentPath(basePath, componentFolderPrefix, component string) string {
	defer perf.Track(nil, "utils.buildComponentPath")()

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
	defer perf.Track(atmosConfig, "utils.getBasePathForComponentType")()

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
	defer perf.Track(atmosConfig, "utils.GetComponentPath")()

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
	defer perf.Track(atmosConfig, "utils.GetComponentBasePath")()

	return GetComponentPath(atmosConfig, componentType, "", "")
}
