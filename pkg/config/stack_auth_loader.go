package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// logKeyIdentity is the log key for identity-related messages.
const logKeyIdentity = "identity"

// stackAuthSection represents the minimal auth section structure for loading.
type stackAuthSection struct {
	Auth struct {
		Identities map[string]struct {
			Default bool `yaml:"default"`
		} `yaml:"identities"`
	} `yaml:"auth"`
}

// stackImportSection represents the import section in a stack file for lightweight parsing.
type stackImportSection struct {
	Import  interface{} `yaml:"import"`
	Imports interface{} `yaml:"imports"`
}

// LoadStackAuthDefaults loads stack configuration files for auth identity defaults.
// This is a lightweight load that doesn't process templates or YAML functions.
// It returns a map of identity names to their default status found in stack configs.
//
// The loader looks for:
//
//	auth:
//	  identities:
//	    <identity-name>:
//	      default: true
//
// This function is used to resolve stack-level default identities before full stack processing,
// solving the chicken-and-egg problem where we need to know the default identity to authenticate,
// but stack configs are only loaded after authentication is configured.
//
// When multiple stack files define DIFFERENT default identities, it means each stack has its own
// default that only applies when that stack is targeted. Since this function runs before stack
// resolution (we don't know the target stack yet), conflicting defaults are discarded to avoid
// false "multiple default identities" errors. The per-stack default will be resolved after full
// stack processing. See https://github.com/cloudposse/atmos/issues/2072.
func LoadStackAuthDefaults(atmosConfig *schema.AtmosConfiguration) (map[string]bool, error) {
	defer perf.Track(atmosConfig, "config.LoadStackAuthDefaults")()

	defaults := make(map[string]bool)

	// If no include paths configured, return empty.
	if len(atmosConfig.IncludeStackAbsolutePaths) == 0 {
		log.Debug("No stack include paths configured, skipping auth default load")
		return defaults, nil
	}

	// Get the top-level stack config files (may exclude _defaults.yaml files via ExcludeStackAbsolutePaths).
	topLevelFiles := getAllStackFiles(atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)

	// Follow import chains from top-level files to discover all files, including imported
	// _defaults.yaml files that may be excluded from IncludeStackAbsolutePaths.
	stackFiles := collectFilesIncludingImports(topLevelFiles, atmosConfig.StacksBaseAbsolutePath)

	log.Debug("Loading stack files for auth defaults", "count", len(stackFiles))

	// Collect all defaults with their source files for conflict detection.
	type defaultSource struct {
		identity string
		file     string
	}
	var allDefaults []defaultSource

	// Load each file for auth defaults.
	for _, filePath := range stackFiles {
		fileDefaults, err := loadFileForAuthDefaults(filePath)
		if err != nil {
			log.Debug("Failed to load file for auth defaults", "file", filePath, "error", err)
			continue // Non-fatal: skip this file.
		}

		for identity, isDefault := range fileDefaults {
			if isDefault {
				allDefaults = append(allDefaults, defaultSource{identity: identity, file: filePath})
				log.Debug("Found default identity in stack config", logKeyIdentity, identity, "file", filePath)
			}
		}
	}

	// If no defaults found, return empty.
	if len(allDefaults) == 0 {
		return defaults, nil
	}

	// Check if all defaults agree on the same identity name.
	// If they do, it's a global default. If they conflict, it means different stacks
	// define different defaults - discard them to avoid false "multiple defaults" errors.
	firstIdentity := allDefaults[0].identity
	allAgree := true
	for _, d := range allDefaults[1:] {
		if d.identity != firstIdentity {
			allAgree = false
			break
		}
	}

	if allAgree {
		// All stack files agree on the same default identity - use it.
		defaults[firstIdentity] = true
		log.Debug("All stack files agree on default identity", logKeyIdentity, firstIdentity, "files", len(allDefaults))
	} else {
		// Conflicting defaults from different stack files.
		// This means each stack has its own default - cannot resolve globally.
		// Return empty so the per-stack default is resolved after full stack processing.
		log.Debug("Conflicting default identities found across stack files, skipping global default resolution",
			"count", len(allDefaults))
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

// collectFilesIncludingImports performs a BFS traversal starting from topLevelFiles,
// following import directives to discover all imported files (e.g., _defaults.yaml files
// that may be excluded from IncludeStackAbsolutePaths but contain auth defaults).
// Returns the deduplicated union of topLevelFiles and all transitively imported files.
func collectFilesIncludingImports(topLevelFiles []string, stacksBasePath string) []string {
	// Use a map to track all visited files and avoid duplicates / cycles.
	visited := make(map[string]bool, len(topLevelFiles))
	result := make([]string, 0, len(topLevelFiles))

	queue := make([]string, 0, len(topLevelFiles))
	for _, f := range topLevelFiles {
		if !visited[f] {
			visited[f] = true
			result = append(result, f)
			queue = append(queue, f)
		}
	}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]

		imports := extractImportPathsFromFile(file)
		for _, rawPath := range imports {
			absPath := resolveImportToAbsPath(rawPath, file, stacksBasePath)
			if absPath == "" || visited[absPath] {
				continue
			}
			// Only include files that actually exist on disk.
			if _, err := os.Stat(absPath); err != nil {
				continue
			}
			visited[absPath] = true
			result = append(result, absPath)
			queue = append(queue, absPath)
		}
	}

	return result
}

// extractImportPathsFromFile reads a YAML file and returns the raw (unresolved) import
// path strings from its `import:` or `imports:` section.
// Both plain-string imports and {path: "..."} object imports are supported.
// If the file cannot be read or parsed, an empty slice is returned (non-fatal).
func extractImportPathsFromFile(filePath string) []string {
	ext := filepath.Ext(filePath)
	if ext != ".yaml" && ext != ".yml" {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var section stackImportSection
	if err := yaml.Unmarshal(content, &section); err != nil {
		// File may contain Go template syntax that prevents YAML parsing.
		// Non-fatal: return empty.
		return nil //nolint:nilerr // Intentional: YAML parse errors are non-fatal.
	}

	var paths []string
	for _, val := range []interface{}{section.Import, section.Imports} {
		paths = append(paths, extractImportPathStrings(val)...)
	}
	return paths
}

// extractImportPathStrings extracts import path strings from an interface{} value.
// Handles: a single string, a slice of strings, and a slice of {path: "..."} objects.
// Entries containing Go template syntax are skipped since they cannot be resolved statically.
func extractImportPathStrings(val interface{}) []string {
	if val == nil {
		return nil
	}

	var results []string

	switch v := val.(type) {
	case string:
		if !containsTemplateSyntax(v) {
			results = append(results, v)
		}
	case []interface{}:
		for _, item := range v {
			switch i := item.(type) {
			case string:
				if !containsTemplateSyntax(i) {
					results = append(results, i)
				}
			case map[string]interface{}:
				// {path: "...", context: {}} import object form.
				if path, ok := i["path"].(string); ok && path != "" && !containsTemplateSyntax(path) {
					results = append(results, path)
				}
			}
		}
	}

	return results
}

// containsTemplateSyntax reports whether s contains Go template delimiters
// ({{ or }}) that would prevent static path resolution.
func containsTemplateSyntax(s string) bool {
	return strings.Contains(s, "{{") || strings.Contains(s, "}}")
}

// resolveImportToAbsPath converts a raw import path string to an absolute file path.
//   - Paths starting with "." or ".." are resolved relative to the parent file's directory.
//   - All other paths are resolved relative to stacksBasePath.
//
// A ".yaml" extension is added when the path has no file extension.
// Returns an empty string if the resolved path is still relative (should not happen in practice).
func resolveImportToAbsPath(rawPath, parentFilePath, stacksBasePath string) string {
	if rawPath == "" {
		return ""
	}

	resolved := u.ResolveRelativePath(rawPath, parentFilePath)

	// ResolveRelativePath returns a non-absolute path for base-path-relative imports.
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(stacksBasePath, resolved)
	}

	// Add .yaml extension when the path has no extension.
	if filepath.Ext(resolved) == "" {
		// Try .yaml first; the caller checks os.Stat so missing files are silently skipped.
		resolved += ".yaml"
	}

	return resolved
}

// loadFileForAuthDefaults reads a single YAML file and extracts auth identity defaults.
// Returns a map of identity name to default status.
func loadFileForAuthDefaults(filePath string) (map[string]bool, error) {
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
