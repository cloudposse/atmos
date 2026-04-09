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

// stackAuthFileWithImports parses the top of a stack file to extract both the
// `import:` list and the `auth:` block. Used by the recursive scanner that
// follows import chains when looking for default identities.
type stackAuthFileWithImports struct {
	Import []any `yaml:"import"`
	Auth   struct {
		Identities map[string]struct {
			Default bool `yaml:"default"`
		} `yaml:"identities"`
	} `yaml:"auth"`
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

	// Get all stack config files.
	stackFiles := getAllStackFiles(atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)

	log.Debug("Loading stack files for auth defaults", "count", len(stackFiles))

	// Collect all defaults with their source files for conflict detection.
	type defaultSource struct {
		identity string
		file     string
	}
	var allDefaults []defaultSource

	// Load each file for auth defaults.
	// Uses the recursive import-following scanner so defaults declared in
	// imported `_defaults.yaml` files are visible even when those files are
	// listed in `excluded_paths` (the excluded-paths filter only prevents
	// standalone processing, not import resolution). This fixes Issue #2293
	// for multi-stack commands that cannot use the exec-layer merge path.
	for _, filePath := range stackFiles {
		// Each top-level stack file gets its own visited set so import cycles
		// in one file tree don't poison another.
		visited := make(map[string]bool)
		fileDefaults := loadAuthWithImports(filePath, atmosConfig.StacksBaseAbsolutePath, visited)

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

// yamlExt / ymlExt are the two stack-file extensions the scanner understands.
// They match the conventions used throughout the Atmos stack processor.
const (
	yamlExt = ".yaml"
	ymlExt  = ".yml"
)

// loadAuthWithImports recursively reads a stack file and its imports, extracting
// auth.identities.*.default markers. The importing file's auth section takes
// precedence over imported files' auth sections when identity names conflict,
// matching Atmos inheritance semantics (more specific wins over more general).
//
// The `visited` map tracks absolute file paths already processed to protect
// against import cycles. `stacksBasePath` is `atmosConfig.StacksBaseAbsolutePath`
// — the root that non-relative imports are resolved against. Relative imports
// (`./foo`, `../bar`) are resolved against the importing file's directory.
//
// Returns nil on any unrecoverable error (unreadable file, YAML parse failure,
// non-YAML extension). Errors are swallowed intentionally — the scanner's
// contract is best-effort discovery, not strict validation.
func loadAuthWithImports(filePath, stacksBasePath string, visited map[string]bool) map[string]bool {
	defer perf.Track(nil, "config.loadAuthWithImports")()

	absPath, ok := markVisited(filePath, visited)
	if !ok {
		return nil
	}

	parsed, ok := readStackAuthFile(absPath)
	if !ok {
		return nil
	}

	result := make(map[string]bool)
	mergeImportedAuthDefaults(parsed.Import, absPath, stacksBasePath, visited, result)
	applyCurrentFileAuthDefaults(parsed, result)
	return result
}

// markVisited returns the absolute path of filePath and true if it is not yet
// in the visited set (marking it in-place), or returns false if the file has
// already been processed (cycle) or the path cannot be resolved.
func markVisited(filePath string, visited map[string]bool) (string, bool) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", false
	}
	if visited[absPath] {
		return "", false
	}
	visited[absPath] = true
	return absPath, true
}

// readStackAuthFile reads a YAML stack file and parses its top-level `import:`
// and `auth:` sections. Returns the parsed struct and true on success, or zero
// value and false for non-YAML extensions, unreadable files, or parse errors
// (which often just mean the file contains Go templates).
func readStackAuthFile(absPath string) (stackAuthFileWithImports, bool) {
	var zero stackAuthFileWithImports

	ext := filepath.Ext(absPath)
	if ext != yamlExt && ext != ymlExt {
		return zero, false
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return zero, false
	}

	var parsed stackAuthFileWithImports
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		// Non-fatal: file may contain Go templates that prevent raw YAML parsing.
		return zero, false
	}
	return parsed, true
}

// mergeImportedAuthDefaults walks each entry in an `import:` list, recursively
// loads the imported files' auth defaults, and merges them into `result`.
// Matches Atmos inheritance order: imported defaults are applied first, then
// the current file overrides via applyCurrentFileAuthDefaults.
func mergeImportedAuthDefaults(
	imports []any,
	importingFile, stacksBasePath string,
	visited map[string]bool,
	result map[string]bool,
) {
	for _, imp := range imports {
		importedFiles := resolveAuthImportPaths(imp, importingFile, stacksBasePath)
		for _, impFile := range importedFiles {
			for name, isDefault := range loadAuthWithImports(impFile, stacksBasePath, visited) {
				if isDefault {
					result[name] = true
				}
			}
		}
	}
}

// applyCurrentFileAuthDefaults applies the default flags declared in the
// current file's `auth.identities` section to `result`. These override any
// imported defaults by virtue of being processed after
// mergeImportedAuthDefaults.
func applyCurrentFileAuthDefaults(parsed stackAuthFileWithImports, result map[string]bool) {
	for name, identity := range parsed.Auth.Identities {
		if identity.Default {
			result[name] = true
		}
	}
}

// resolveAuthImportPaths resolves a single `import:` list entry to absolute file
// paths on disk. Handles the three Atmos import forms:
//
//  1. Plain string: "orgs/acme/_defaults" — resolved against `stacksBasePath`.
//  2. Glob string:  "mixins/region/*"    — resolved against `stacksBasePath`,
//     then glob-expanded via `GetGlobMatches` (supports doublestar `**`).
//  3. Map form with `path:` field:
//     `- path: orgs/acme/_defaults` — the `path` value is treated as a string.
//
// Relative paths (`./foo`, `../bar`) are resolved against the importing file's
// directory instead of `stacksBasePath`.
//
// Returns nil for import forms that cannot be resolved without running Go
// templates (e.g., `path: "{{ .something }}"`), for paths that do not exist
// on disk, or for any other error. This is intentional graceful degradation —
// the scanner is best-effort and never blocks the auth flow on a malformed or
// templated import.
func resolveAuthImportPaths(imp any, importingFile, stacksBasePath string) []string {
	defer perf.Track(nil, "config.resolveAuthImportPaths")()

	importPath := normalizeImportPath(extractImportPathString(imp))
	if importPath == "" {
		return nil
	}

	candidate, ok := buildImportCandidatePath(importPath, importingFile, stacksBasePath)
	if !ok {
		return nil
	}

	return matchImportCandidate(candidate)
}

// normalizeImportPath returns the raw import path with a `.yaml` extension
// appended if missing, or empty string if the input is empty or contains a Go
// template expression (which cannot be resolved without template context).
func normalizeImportPath(rawPath string) string {
	if rawPath == "" {
		return ""
	}
	if strings.Contains(rawPath, "{{") {
		return ""
	}
	if filepath.Ext(rawPath) == "" {
		rawPath += yamlExt
	}
	return rawPath
}

// buildImportCandidatePath joins the normalized import path against either the
// importing file's directory (for `./` / `../` relative imports) or the stacks
// base path (for everything else). Returns the candidate path and true, or
// empty/false if the base path required for resolution is missing.
func buildImportCandidatePath(importPath, importingFile, stacksBasePath string) (string, bool) {
	normalized := filepath.FromSlash(importPath)
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return filepath.Join(filepath.Dir(importingFile), normalized), true
	}
	if stacksBasePath == "" {
		return "", false
	}
	return filepath.Join(stacksBasePath, normalized), true
}

// matchImportCandidate returns matching file paths for a candidate. For glob
// patterns it uses doublestar; for plain paths it uses a direct stat, with a
// `.yml` fallback when the original `.yaml` candidate does not exist.
func matchImportCandidate(candidate string) []string {
	if strings.ContainsAny(candidate, "*?[") {
		matches, err := u.GetGlobMatches(candidate)
		if err != nil || len(matches) == 0 {
			return nil
		}
		return matches
	}

	if _, err := os.Stat(candidate); err == nil {
		return []string{candidate}
	}

	// Fall back to .yml if .yaml did not exist.
	if strings.HasSuffix(candidate, yamlExt) {
		alt := strings.TrimSuffix(candidate, yamlExt) + ymlExt
		if _, err := os.Stat(alt); err == nil {
			return []string{alt}
		}
	}

	return nil
}

// extractImportPathString extracts the path string from an import list entry.
// Supports plain string imports and map-form imports with a `path` key.
// Returns empty string for unrecognized forms — the caller treats that as
// unresolvable and skips the import gracefully.
func extractImportPathString(imp any) string {
	switch v := imp.(type) {
	case string:
		return v
	case map[string]any:
		if p, ok := v["path"].(string); ok {
			return p
		}
	case map[any]any:
		// yaml.v3 sometimes decodes into map[any]any depending on tag state.
		if p, ok := v["path"].(string); ok {
			return p
		}
	}
	return ""
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
