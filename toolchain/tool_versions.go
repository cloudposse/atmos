package toolchain

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Define an interface for Resolve for testability
// This allows both *Installer and *fakeInstaller to be used.
type toolNameResolver interface {
	Resolve(toolName string) (string, string, error)
}

// ToolVersions represents the .tool-versions file format (asdf-compatible: tool -> list of versions, first is default).
type ToolVersions struct {
	Tools map[string][]string
}

// LoadToolVersions loads a .tool-versions file (asdf-compatible: tool version1 [version2 ...]).
func LoadToolVersions(filePath string) (*ToolVersions, error) {
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
			return nil, fmt.Errorf("invalid format at line %d: '%s' (missing version)", i+1, line)
		}
		tool := parts[0]
		versions := parts[1:]
		toolVersions.Tools[tool] = append(toolVersions.Tools[tool], versions...)
	}

	return toolVersions, nil
}

// SaveToolVersions saves a ToolVersions struct to a .tool-versions file (asdf-compatible).
func SaveToolVersions(filePath string, toolVersions *ToolVersions) error {
	if toolVersions == nil || toolVersions.Tools == nil {
		return fmt.Errorf("toolVersions or toolVersions.Tools is nil")
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
				return fmt.Errorf("tool '%s' is missing a version", tool)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s", tool, strings.Join(versions, " ")))
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}

// AddVersionToTool adds a version to a tool, optionally as default (front of list).
func AddVersionToTool(toolVersions *ToolVersions, tool, version string, asDefault bool) {
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

// RemoveVersionFromTool removes a version from a tool.
func RemoveVersionFromTool(toolVersions *ToolVersions, tool, version string) {
	versions := toolVersions.Tools[tool]
	newVersions := make([]string, 0, len(versions))
	for _, v := range versions {
		if v != version {
			newVersions = append(newVersions, v)
		}
	}
	if len(newVersions) == 0 {
		delete(toolVersions.Tools, tool)
	} else {
		toolVersions.Tools[tool] = newVersions
	}
}

// GetDefaultVersion returns the default (first) version for a tool.
func GetDefaultVersion(toolVersions *ToolVersions, tool string) (string, bool) {
	versions := toolVersions.Tools[tool]
	if len(versions) == 0 {
		return "", false
	}
	return versions[0], true
}

// GetAllVersions returns all versions for a tool.
func GetAllVersions(toolVersions *ToolVersions, tool string) []string {
	return toolVersions.Tools[tool]
}

// AddToolToVersions adds a tool/version combination to the .tool-versions file
// If the tool already exists, it updates the version
// If the aliased version already exists, it skips adding the non-aliased version to prevent duplicates.
func AddToolToVersions(filePath, tool, version string) error {
	return addToolToVersionsInternal(filePath, tool, version, false)
}

// AddToolToVersionsAsDefault adds a tool and version to the .tool-versions file as the default
// If the tool already exists, it updates the version and sets it as default.
func AddToolToVersionsAsDefault(filePath, tool, version string) error {
	return addToolToVersionsInternal(filePath, tool, version, true)
}

// addToolToVersionsInternal is the single path for adding tools to .tool-versions
// All other functions should use this to ensure consistent duplicate checking.
func addToolToVersionsInternal(filePath, tool, version string, asDefault bool) error {
	if version == "" {
		return fmt.Errorf("cannot add tool '%s' without a version", tool)
	}
	// Load existing tool versions
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		// If file doesn't exist, create a new one
		if os.IsNotExist(err) {
			toolVersions = &ToolVersions{
				Tools: make(map[string][]string),
			}
		} else {
			return fmt.Errorf("failed to load existing .tool-versions: %w", err)
		}
	}

	// Check if this would create a duplicate with an aliased version
	if wouldCreateDuplicate(toolVersions, tool, version) {
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
func wouldCreateDuplicate(toolVersions *ToolVersions, tool, version string) bool {
	// Create an installer to use its resolver
	installer := NewInstaller()

	// Check if the tool is an alias (e.g., "opentofu")
	owner, repo, err := installer.parseToolSpec(tool)
	if err == nil && owner != "" && repo != "" {
		// This is an alias, check if the full name already exists
		aliasKey := owner + "/" + repo
		if versions, ok := toolVersions.Tools[aliasKey]; ok {
			for _, v := range versions {
				if v == version {
					return true // Duplicate found
				}
			}
		}
	}

	// Check if this is a full name (e.g., "opentofu/opentofu") and if the alias exists
	// We need to check if there's an alias that resolves to this full name
	for existingTool, versions := range toolVersions.Tools {
		// Skip if it's the same tool
		if existingTool == tool {
			continue
		}

		// Try to resolve the existing tool as an alias
		existingOwner, existingRepo, err := installer.parseToolSpec(existingTool)
		if err == nil && existingOwner != "" && existingRepo != "" {
			existingAliasKey := existingOwner + "/" + existingRepo
			if existingAliasKey == tool {
				// The existing tool is an alias for this full name
				for _, v := range versions {
					if v == version {
						return true // Duplicate found
					}
				}
			}
		}
	}

	return false
}

// RemoveToolFromVersions removes a tool from the .tool-versions file.
func RemoveToolFromVersions(filePath, tool string) error {
	// Load existing tool versions
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to load existing .tool-versions: %w", err)
	}

	// Remove the tool entirely
	delete(toolVersions.Tools, tool)

	// Save back to file
	return SaveToolVersions(filePath, toolVersions)
}

// GetToolVersion gets the version for a specific tool from the .tool-versions file.
func GetToolVersion(filePath, tool string) (string, bool, error) {
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return "", false, err
	}

	versions := toolVersions.Tools[tool]
	if len(versions) == 0 {
		return "", false, nil
	}
	return versions[0], true, nil
}

// HasToolVersion checks if a tool/version combination exists in the .tool-versions file.
func HasToolVersion(filePath, tool, version string) (bool, error) {
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return false, err
	}

	versions := toolVersions.Tools[tool]
	for _, v := range versions {
		if v == version {
			return true, nil
		}
	}
	return false, nil
}

// ParseToolVersionsLine parses a single line from a .tool-versions file
// Returns tool name, version, and whether the line is valid.
func ParseToolVersionsLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	// Only allow exactly one space, no tabs or multiple spaces
	if strings.Count(line, " ") != 1 || strings.Contains(line, "\t") {
		return "", "", false
	}
	parts := strings.Split(line, " ")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

// ValidateToolVersionsFile validates the format of a .tool-versions file.
func ValidateToolVersionsFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lineNum := i + 1
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the line
		_, _, valid := ParseToolVersionsLine(line)
		if !valid {
			return fmt.Errorf("invalid format at line %d: '%s' (must be <tool> <version> with exactly one space)", lineNum, line)
		}
	}

	return nil
}

// GetToolVersionsFileContent returns the raw content of a .tool-versions file.
func GetToolVersionsFileContent(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MergeToolVersions merges two ToolVersions structs, with the second one taking precedence.
func MergeToolVersions(base, override *ToolVersions) *ToolVersions {
	result := &ToolVersions{
		Tools: make(map[string][]string),
	}

	// Copy base tools
	for tool, versions := range base.Tools {
		result.Tools[tool] = append([]string{}, versions...) // Deep copy
	}

	// Override with override tools
	for tool, versions := range override.Tools {
		result.Tools[tool] = append([]string{}, versions...) // Deep copy
	}

	return result
}

// LookupToolVersion attempts to find the version for a tool, trying both the raw name and its resolved alias.
// Returns the key found (raw or alias), the version, and whether it was found.
func LookupToolVersion(tool string, toolVersions *ToolVersions, resolver toolNameResolver) (resolvedKey, version string, found bool) {
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

// LookupToolVersionOrLatest attempts to find the version for a tool, trying both the raw name and its resolved alias.
// If not found, but the alias resolves, returns 'latest' as the version and usedLatest=true.
// Returns the key found (raw or alias), the version, whether it was found in toolVersions, and whether 'latest' was used as a fallback.
func LookupToolVersionOrLatest(tool string, toolVersions *ToolVersions, resolver toolNameResolver) (resolvedKey, version string, found bool, usedLatest bool) {
	// Try raw tool name first
	if versions, ok := toolVersions.Tools[tool]; ok && len(versions) > 0 {
		return tool, versions[0], true, false
	}
	// Try alias resolution
	owner, repo, err := resolver.Resolve(tool)
	if err == nil {
		aliasKey := owner + "/" + repo
		if versions, ok := toolVersions.Tools[aliasKey]; ok && len(versions) > 0 {
			return aliasKey, versions[0], true, false
		}
		// Alias resolves, but not in toolVersions: fallback to latest
		return aliasKey, "latest", false, true
	}
	return "", "", false, false
}

// ParseToolVersionArg parses a CLI argument in the form tool, tool@version, or owner/repo@version.
// Returns tool (or owner/repo), version (may be empty), and error if invalid.
func ParseToolVersionArg(arg string) (string, string, error) {
	if arg == "" {
		return "", "", fmt.Errorf("empty tool argument")
	}
	if strings.Count(arg, "@") > 1 {
		return "", "", fmt.Errorf("invalid tool specification: %q (multiple @)", arg)
	}
	parts := strings.SplitN(arg, "@", 2)
	if len(parts) == 1 {
		if parts[0] == "" {
			return "", "", fmt.Errorf("invalid tool specification: %q (missing tool name)", arg)
		}
		return parts[0], "", nil
	}
	if parts[0] == "" {
		return "", "", fmt.Errorf("invalid tool specification: %q (missing tool name before @)", arg)
	}
	if parts[1] == "" {
		return "", "", fmt.Errorf("invalid tool specification: %q (missing version after @)", arg)
	}
	return parts[0], parts[1], nil
}
