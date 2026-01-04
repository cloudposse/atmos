package toolchain

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ToolVersions represents the .tool-versions file format (asdf-compatible: tool -> list of versions, first is default).
type ToolVersions struct {
	Tools map[string][]string
}

// LoadToolVersions loads a .tool-versions file (asdf-compatible: tool version1 [version2 ...]).
func LoadToolVersions(filePath string) (*ToolVersions, error) {
	defer perf.Track(nil, "toolchain.LoadToolVersions")()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	toolVersions := &ToolVersions{
		Tools: make(map[string][]string),
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("%w: invalid format at line %d: '%s' (missing version)", ErrInvalidToolSpec, i+1, line)
		}
		tool := parts[0]
		versions := parts[1:]
		toolVersions.Tools[tool] = append(toolVersions.Tools[tool], versions...)
	}

	return toolVersions, nil
}

// SaveToolVersions saves a ToolVersions struct to a .tool-versions file (asdf-compatible).
func SaveToolVersions(filePath string, toolVersions *ToolVersions) error {
	defer perf.Track(nil, "toolchain.SaveToolVersions")()

	if toolVersions == nil || toolVersions.Tools == nil {
		return fmt.Errorf("%w: toolVersions or toolVersions.Tools is nil", ErrInvalidToolSpec)
	}
	var lines []string

	// Sort tool names for deterministic output
	var toolNames []string
	for tool := range toolVersions.Tools {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)

	for _, tool := range toolNames {
		versions := toolVersions.Tools[tool]
		if len(versions) == 0 {
			continue
		}
		for _, v := range versions {
			if v == "" {
				return fmt.Errorf("%w: tool '%s' is missing a version", ErrInvalidToolSpec, tool)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s", tool, strings.Join(versions, " ")))
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), defaultFileWritePermissions)
}

// AddVersionToTool adds a version to a tool, optionally as default (front of list).
func AddVersionToTool(toolVersions *ToolVersions, tool, version string, asDefault bool) {
	defer perf.Track(nil, "toolchain.AddVersionToTool")()

	// Guard against nil map.
	if toolVersions.Tools == nil {
		toolVersions.Tools = make(map[string][]string)
	}

	versions := toolVersions.Tools[tool]
	for i, v := range versions {
		if v == version {
			if asDefault && i != 0 {
				// Move to front
				versions = append([]string{version}, append(versions[:i], versions[i+1:]...)...)
				toolVersions.Tools[tool] = versions
			}
			return
		}
	}
	if asDefault {
		toolVersions.Tools[tool] = append([]string{version}, versions...)
	} else {
		toolVersions.Tools[tool] = append(versions, version)
	}
}

// GetDefaultVersion returns the default (first) version for a tool.
func GetDefaultVersion(toolVersions *ToolVersions, tool string) (string, bool) {
	defer perf.Track(nil, "toolchain.GetDefaultVersion")()

	versions := toolVersions.Tools[tool]
	if len(versions) == 0 {
		return "", false
	}
	return versions[0], true
}

// GetAllVersions returns all versions for a tool.
func GetAllVersions(toolVersions *ToolVersions, tool string) []string {
	defer perf.Track(nil, "toolchain.GetAllVersions")()

	return toolVersions.Tools[tool]
}

// AddToolToVersions adds a tool/version combination to the .tool-versions file
// If the tool already exists, it updates the version
// If the aliased version already exists, it skips adding the non-aliased version to prevent duplicates.
func AddToolToVersions(filePath, tool, version string) error {
	defer perf.Track(nil, "toolchain.AddToolToVersions")()

	return addToolToVersionsInternal(filePath, tool, version, false)
}

// AddToolToVersionsAsDefault adds a tool and version to the .tool-versions file as the default
// If the tool already exists, it updates the version and sets it as default.
func AddToolToVersionsAsDefault(filePath, tool, version string) error {
	defer perf.Track(nil, "toolchain.AddToolToVersionsAsDefault")()

	return addToolToVersionsInternal(filePath, tool, version, true)
}

// addToolToVersionsInternal is the single path for adding tools to .tool-versions
// All other functions should use this to ensure consistent duplicate checking.
func addToolToVersionsInternal(filePath, tool, version string, asDefault bool) error {
	defer perf.Track(nil, "toolchain.addToolToVersionsInternal")()

	if version == "" {
		return fmt.Errorf("%w: cannot add tool '%s' without a version", ErrInvalidToolSpec, tool)
	}
	// Load existing tool versions
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		// If file doesn't exist, create a new one
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load existing .tool-versions: %w", err)
		}
		toolVersions = &ToolVersions{
			Tools: make(map[string][]string),
		}
	}

	// Create an installer to use its resolver
	installer := NewInstaller()
	resolver := installer.GetResolver()

	// Check if this would create a duplicate with an aliased version
	if wouldCreateDuplicate(toolVersions, tool, version, resolver) {
		// Skip adding this entry as it would create a duplicate
		return nil
	}

	// Add or update the tool
	AddVersionToTool(toolVersions, tool, version, asDefault)

	// Save back to file
	return SaveToolVersions(filePath, toolVersions)
}

// wouldCreateDuplicate checks if adding a tool/version combination would create a duplicate
// with an existing aliased version. For example, if "opentofu/opentofu 1.10.3" already exists,
// adding "opentofu 1.10.3" would create a duplicate.
func wouldCreateDuplicate(toolVersions *ToolVersions, tool, version string, resolver ToolResolver) bool {
	// Check if the tool is an alias that conflicts with an existing full name.
	if aliasConflictsWithFullName(toolVersions, tool, version, resolver) {
		return true
	}

	// Check if the tool is a full name that conflicts with an existing alias.
	if fullNameConflictsWithAlias(toolVersions, tool, version, resolver) {
		return true
	}

	return false
}

// aliasConflictsWithFullName checks if an alias conflicts with an existing full name entry.
// For example, if "opentofu/opentofu 1.10.3" already exists, adding "opentofu 1.10.3" would be a duplicate.
func aliasConflictsWithFullName(toolVersions *ToolVersions, tool, version string, resolver ToolResolver) bool {
	// Check if the tool is an alias (e.g., "opentofu").
	owner, repo, err := resolver.Resolve(tool)
	if err != nil || owner == "" || repo == "" {
		return false
	}

	// This is an alias, check if the full name already exists.
	aliasKey := owner + "/" + repo
	versions, ok := toolVersions.Tools[aliasKey]
	if !ok {
		return false
	}

	// Check if any existing version matches.
	for _, v := range versions {
		if v == version {
			return true // Duplicate found.
		}
	}

	return false
}

// fullNameConflictsWithAlias checks if a full name conflicts with an existing alias entry.
// For example, if "opentofu 1.10.3" already exists, adding "opentofu/opentofu 1.10.3" would be a duplicate.
func fullNameConflictsWithAlias(toolVersions *ToolVersions, tool, version string, resolver ToolResolver) bool {
	// Check if this is a full name (e.g., "opentofu/opentofu") and if an alias exists
	// that resolves to this full name.
	for existingTool, versions := range toolVersions.Tools {
		// Skip if it's the same tool.
		if existingTool == tool {
			continue
		}

		// Try to resolve the existing tool as an alias.
		existingOwner, existingRepo, err := resolver.Resolve(existingTool)
		if err != nil || existingOwner == "" || existingRepo == "" {
			continue
		}

		existingAliasKey := existingOwner + "/" + existingRepo
		if existingAliasKey != tool {
			continue
		}

		// The existing tool is an alias for this full name.
		// Check if any version matches.
		for _, v := range versions {
			if v == version {
				return true // Duplicate found.
			}
		}
	}

	return false
}

// LookupToolVersion attempts to find the version for a tool, trying both the raw name and its resolved alias.
// Returns the key found (raw or alias), the version, and whether it was found.
func LookupToolVersion(tool string, toolVersions *ToolVersions, resolver ToolResolver) (resolvedKey, version string, found bool) {
	defer perf.Track(nil, "toolchain.LookupToolVersion")()

	// Try raw tool name first
	if versions, ok := toolVersions.Tools[tool]; ok && len(versions) > 0 {
		return tool, versions[0], true
	}
	// Try alias resolution
	owner, repo, err := resolver.Resolve(tool)
	if err == nil {
		aliasKey := owner + "/" + repo
		if versions, ok := toolVersions.Tools[aliasKey]; ok && len(versions) > 0 {
			return aliasKey, versions[0], true
		}
	}
	return "", "", false
}

// ToolVersionLookupResult holds the result of looking up a tool version.
type ToolVersionLookupResult struct {
	ResolvedKey string // The key found (raw tool name or owner/repo alias).
	Version     string // The version string.
	Found       bool   // Whether the tool was found in toolVersions.
	UsedLatest  bool   // Whether 'latest' was used as a fallback.
}

// LookupToolVersionOrLatest attempts to find the version for a tool, trying both the raw name and its resolved alias.
// If not found, but the alias resolves, returns 'latest' as the version and usedLatest=true.
func LookupToolVersionOrLatest(tool string, toolVersions *ToolVersions, resolver ToolResolver) ToolVersionLookupResult {
	defer perf.Track(nil, "toolchain.LookupToolVersionOrLatest")()

	// Try raw tool name first
	if versions, ok := toolVersions.Tools[tool]; ok && len(versions) > 0 {
		return ToolVersionLookupResult{ResolvedKey: tool, Version: versions[0], Found: true, UsedLatest: false}
	}
	// Try alias resolution
	owner, repo, err := resolver.Resolve(tool)
	if err == nil {
		aliasKey := owner + "/" + repo
		if versions, ok := toolVersions.Tools[aliasKey]; ok && len(versions) > 0 {
			return ToolVersionLookupResult{ResolvedKey: aliasKey, Version: versions[0], Found: true, UsedLatest: false}
		}
		// Alias resolves, but not in toolVersions: fallback to latest
		return ToolVersionLookupResult{ResolvedKey: aliasKey, Version: "latest", Found: false, UsedLatest: true}
	}
	return ToolVersionLookupResult{ResolvedKey: "", Version: "", Found: false, UsedLatest: false}
}

// ParseToolVersionArg parses a CLI argument in the form tool, tool@version, or owner/repo@version.
// Returns tool (or owner/repo), version (may be empty), and error if invalid.
func ParseToolVersionArg(arg string) (string, string, error) {
	defer perf.Track(nil, "toolchain.ParseToolVersionArg")()

	if arg == "" {
		return "", "", fmt.Errorf("%w: empty tool argument", ErrInvalidToolSpec)
	}
	if strings.Count(arg, "@") > 1 {
		return "", "", fmt.Errorf("%w: %q (multiple @)", ErrInvalidToolSpec, arg)
	}
	parts := strings.SplitN(arg, "@", 2)
	if len(parts) == 1 {
		if parts[0] == "" {
			return "", "", fmt.Errorf("%w: %q (missing tool name)", ErrInvalidToolSpec, arg)
		}
		return parts[0], "", nil
	}
	if parts[0] == "" {
		return "", "", fmt.Errorf("%w: %q (missing tool name before @)", ErrInvalidToolSpec, arg)
	}
	if parts[1] == "" {
		return "", "", fmt.Errorf("%w: %q (missing version after @)", ErrInvalidToolSpec, arg)
	}
	return parts[0], parts[1], nil
}

// RemoveToolFromVersions removes a tool from the .tool-versions file.
func RemoveToolFromVersions(filePath, tool, version string) error {
	defer perf.Track(nil, "toolchain.RemoveToolFromVersions")()

	// Load existing tool versions
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	// Remove the tool entirely
	delete(toolVersions.Tools, tool)

	// Save back to file
	return SaveToolVersions(filePath, toolVersions)
}
