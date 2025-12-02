package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// logKeyIdentity is the log key for identity-related messages.
const logKeyIdentity = "identity"

// stackAuthSection represents the minimal auth section structure for scanning.
type stackAuthSection struct {
	Auth struct {
		Identities map[string]struct {
			Default bool `yaml:"default"`
		} `yaml:"identities"`
	} `yaml:"auth"`
}

// ScanStackAuthDefaults scans stack configuration files for auth identity defaults.
// This is a lightweight scan that doesn't process templates or YAML functions.
// It returns a map of identity names to their default status found in stack configs.
//
// The scan looks for:
//
//	auth:
//	  identities:
//	    <identity-name>:
//	      default: true
//
// This function is used to resolve stack-level default identities before full stack processing,
// solving the chicken-and-egg problem where we need to know the default identity to authenticate,
// but stack configs are only loaded after authentication is configured.
func ScanStackAuthDefaults(atmosConfig *schema.AtmosConfiguration) (map[string]bool, error) {
	defer perf.Track(atmosConfig, "config.ScanStackAuthDefaults")()

	defaults := make(map[string]bool)

	// If no include paths configured, return empty.
	if len(atmosConfig.IncludeStackAbsolutePaths) == 0 {
		log.Debug("No stack include paths configured, skipping auth default scan")
		return defaults, nil
	}

	// Get all stack config files.
	stackFiles := getAllStackFiles(atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)

	log.Debug("Scanning stack files for auth defaults", "count", len(stackFiles))

	// Scan each file for auth defaults.
	for _, filePath := range stackFiles {
		fileDefaults, err := scanFileForAuthDefaults(filePath)
		if err != nil {
			log.Debug("Failed to scan file for auth defaults", "file", filePath, "error", err)
			continue // Non-fatal: skip this file.
		}

		// Merge found defaults (later files can override earlier ones).
		for identity, isDefault := range fileDefaults {
			if isDefault {
				defaults[identity] = true
				log.Debug("Found default identity in stack config", logKeyIdentity, identity, "file", filePath)
			}
		}
	}

	return defaults, nil
}

// getAllStackFiles returns all stack config file paths matching the include patterns
// and not matching exclude patterns.
func getAllStackFiles(includePaths, excludePaths []string) []string {
	var allFiles []string

	for _, pattern := range includePaths {
		matches, err := u.GetGlobMatches(pattern)
		if err != nil {
			continue // Non-fatal: skip invalid patterns.
		}
		allFiles = append(allFiles, matches...)
	}

	// Filter out excluded paths.
	if len(excludePaths) == 0 {
		return allFiles
	}

	var filtered []string
	excludeMap := make(map[string]bool)

	// Build a set of excluded files.
	for _, pattern := range excludePaths {
		matches, err := u.GetGlobMatches(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			excludeMap[match] = true
		}
	}

	// Filter out excluded files.
	for _, file := range allFiles {
		if !excludeMap[file] {
			filtered = append(filtered, file)
		}
	}

	return filtered
}

// scanFileForAuthDefaults reads a single YAML file and extracts auth identity defaults.
// Returns a map of identity name to default status.
func scanFileForAuthDefaults(filePath string) (map[string]bool, error) {
	defaults := make(map[string]bool)

	// Only process YAML files.
	ext := filepath.Ext(filePath)
	if ext != ".yaml" && ext != ".yml" {
		return defaults, nil
	}

	// Read file contents.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Parse YAML (minimal parsing, no template processing).
	var section stackAuthSection
	if err := yaml.Unmarshal(content, &section); err != nil {
		// Non-fatal: file may have templates that prevent parsing.
		// Return empty defaults without error - this is expected for files with Go templates.
		return defaults, nil //nolint:nilerr // Intentional: YAML parse errors are non-fatal for template files.
	}

	// Extract defaults from auth.identities.
	for name, identity := range section.Auth.Identities {
		if identity.Default {
			defaults[name] = true
		}
	}

	return defaults, nil
}

// MergeStackAuthDefaults merges stack-level auth defaults into the auth config.
// Stack defaults have HIGHER priority than atmos.yaml defaults (following Atmos inheritance model).
// This means stack config can override the default identity set in atmos.yaml.
func MergeStackAuthDefaults(authConfig *schema.AuthConfig, stackDefaults map[string]bool) {
	defer perf.Track(nil, "config.MergeStackAuthDefaults")()

	if authConfig == nil || len(stackDefaults) == 0 {
		return
	}

	// Stack config takes precedence over atmos.yaml (following Atmos inheritance model).
	// First, clear any existing defaults from atmos.yaml if stack has defaults.
	if hasAnyDefault(stackDefaults) {
		clearExistingDefaults(authConfig)
	}

	// Apply stack-level defaults.
	applyStackDefaults(authConfig, stackDefaults)
}

// hasAnyDefault checks if any identity in the defaults map has default: true.
func hasAnyDefault(defaults map[string]bool) bool {
	for _, isDefault := range defaults {
		if isDefault {
			return true
		}
	}
	return false
}

// clearExistingDefaults removes the default flag from all identities in the auth config.
// This is used when stack config has defaults, which take precedence over atmos.yaml.
func clearExistingDefaults(authConfig *schema.AuthConfig) {
	for identityName, identity := range authConfig.Identities {
		if identity.Default {
			identity.Default = false
			authConfig.Identities[identityName] = identity
			log.Debug("Cleared atmos.yaml default (stack takes precedence)", logKeyIdentity, identityName)
		}
	}
}

// applyStackDefaults applies stack-level default flags to matching identities in the auth config.
func applyStackDefaults(authConfig *schema.AuthConfig, stackDefaults map[string]bool) {
	for identityName, isDefault := range stackDefaults {
		if !isDefault {
			continue
		}

		identity, exists := authConfig.Identities[identityName]
		if !exists {
			// Identity doesn't exist in atmos.yaml config - skip.
			// Stack defaults only set the default flag, they don't create identities.
			log.Debug("Stack default identity not found in atmos config", logKeyIdentity, identityName)
			continue
		}

		identity.Default = true
		authConfig.Identities[identityName] = identity
		log.Debug("Applied stack-level default to identity", logKeyIdentity, identityName)
	}
}
