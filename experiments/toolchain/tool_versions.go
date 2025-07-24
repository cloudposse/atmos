package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Define an interface for Resolve for testability
// This allows both *Installer and *fakeInstaller to be used
type toolNameResolver interface {
	Resolve(toolName string) (string, string, error)
}

var toolVersionsCmd = &cobra.Command{
	Use:   "tool-versions [file]",
	Short: "List tools from .tool-versions file and their install status",
	Long: `List all tools specified in a .tool-versions file and show their install status.

Examples:
  toolchain tool-versions                    # Use .tool-versions in current directory
  toolchain tool-versions .tool-versions     # Use specific file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runToolVersions,
}

var toolVersionsAddCmd = &cobra.Command{
	Use:   "add <tool> <version>",
	Short: "Add or update a tool and version in .tool-versions",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = ".tool-versions"
		}
		tool := args[0]
		version := args[1]
		err := AddToolToVersions(filePath, tool, version)
		if err != nil {
			return err
		}
		fmt.Printf("%s Added/updated %s %s in %s\n", checkMark.Render(), tool, version, filePath)
		return nil
	},
}

var toolVersionsRemoveCmd = &cobra.Command{
	Use:   "remove <tool>",
	Short: "Remove a tool from .tool-versions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = ".tool-versions"
		}
		tool := args[0]
		err := RemoveToolFromVersions(filePath, tool)
		if err != nil {
			return err
		}
		fmt.Printf("✅ Removed %s from %s\n", tool, filePath)
		return nil
	},
}

func init() {
	toolVersionsCmd.AddCommand(toolVersionsAddCmd)
	toolVersionsCmd.AddCommand(toolVersionsRemoveCmd)
	toolVersionsAddCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
	toolVersionsRemoveCmd.Flags().String("file", ".tool-versions", "Path to .tool-versions file")
}

func runToolVersions(cmd *cobra.Command, args []string) error {
	filePath := ".tool-versions"
	if len(args) > 0 {
		filePath = args[0]
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Load tool versions
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	installer := NewInstaller()

	fmt.Printf("📋 Tools from %s:\n", filePath)
	fmt.Printf("%-30s %-15s\n", "Tool", "Version")
	fmt.Printf("%s\n", strings.Repeat("-", 50))

	for tool, versions := range toolVersions.Tools {
		// Parse tool specification (owner/repo@version or just repo@version)
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			fmt.Printf("%-30s %-15s %s\n", tool, versions[0], xMark.Render())
			continue
		}

		// Check if installed
		_, err = installer.findBinaryPath(owner, repo, versions[0])
		status := xMark.Render()
		if err == nil {
			status = checkMark.Render()
		}

		fmt.Printf("%-30s %-15s %s\n", tool, versions[0], status)
	}

	return nil
}

// ToolVersions represents the .tool-versions file format (asdf-compatible: tool -> list of versions, first is default)
type ToolVersions struct {
	Tools map[string][]string
}

// LoadToolVersions loads a .tool-versions file (asdf-compatible: tool version1 [version2 ...])
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

// SaveToolVersions saves a ToolVersions struct to a .tool-versions file (asdf-compatible)
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
	return os.WriteFile(filePath, []byte(content), 0644)
}

// AddVersionToTool adds a version to a tool, optionally as default (front of list)
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

// RemoveVersionFromTool removes a version from a tool
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

// GetDefaultVersion returns the default (first) version for a tool
func GetDefaultVersion(toolVersions *ToolVersions, tool string) (string, bool) {
	versions := toolVersions.Tools[tool]
	if len(versions) == 0 {
		return "", false
	}
	return versions[0], true
}

// GetAllVersions returns all versions for a tool
func GetAllVersions(toolVersions *ToolVersions, tool string) []string {
	return toolVersions.Tools[tool]
}

// AddToolToVersions adds a tool/version combination to the .tool-versions file
// If the tool already exists, it updates the version
func AddToolToVersions(filePath, tool, version string) error {
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

	// Add or update the tool
	AddVersionToTool(toolVersions, tool, version, false)

	// Save back to file
	return SaveToolVersions(filePath, toolVersions)
}

// RemoveToolFromVersions removes a tool from the .tool-versions file
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

// GetToolVersion gets the version for a specific tool from the .tool-versions file
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

// HasToolVersion checks if a tool/version combination exists in the .tool-versions file
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
// Returns tool name, version, and whether the line is valid
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

// ValidateToolVersionsFile validates the format of a .tool-versions file
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

// GetToolVersionsFileContent returns the raw content of a .tool-versions file
func GetToolVersionsFileContent(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MergeToolVersions merges two ToolVersions structs, with the second one taking precedence
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
