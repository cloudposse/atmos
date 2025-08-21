package toolchain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ListToolVersions handles the logic for listing tool versions.
func ListToolVersions(showAll bool, limit int, toolName string) error {
	filePath := atmosConfig.Toolchain.FilePath
	// Resolve the tool name to handle aliases
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}
	resolvedKey := owner + "/" + repo

	var versions []string
	var defaultVersion string

	if showAll {
		// Fetch all available versions from GitHub
		allVersions, err := fetchAllGitHubVersions(owner, repo, limit)
		if err != nil {
			return fmt.Errorf("failed to fetch versions from GitHub: %w", err)
		}
		versions = allVersions

		// Load tool versions to get the default
		toolVersions, err := LoadToolVersions(filePath)
		if err == nil {
			if configuredVersions, exists := toolVersions.Tools[resolvedKey]; exists && len(configuredVersions) > 0 {
				defaultVersion = configuredVersions[0]
			} else if configuredVersions, exists := toolVersions.Tools[toolName]; exists && len(configuredVersions) > 0 {
				defaultVersion = configuredVersions[0]
			}
		}
	} else {
		// Load tool versions from file
		toolVersions, err := LoadToolVersions(filePath)
		if err != nil {
			return fmt.Errorf("failed to load .tool-versions: %w", err)
		}

		// Get versions for the tool - try both resolved key and original tool name
		fileVersions, exists := toolVersions.Tools[resolvedKey]
		if !exists {
			fileVersions, exists = toolVersions.Tools[toolName]
			if !exists {
				return fmt.Errorf("tool '%s' not found in %s", toolName, filePath)
			}
		}

		if len(fileVersions) == 0 {
			return fmt.Errorf("no versions configured for tool '%s' in %s", toolName, filePath)
		}

		versions = fileVersions
		defaultVersion = versions[0]
	}

	// Deduplicate versions
	seen := make(map[string]bool)
	uniqueVersions := []string{}
	for _, version := range versions {
		if !seen[version] {
			seen[version] = true
			uniqueVersions = append(uniqueVersions, version)
		}
	}
	versions = uniqueVersions

	// Sort versions in semver order
	sortedVersions, err := sortVersionsSemver(versions)
	if err != nil {
		// Fall back to string sorting
		sort.Strings(versions)
		sortedVersions = versions
	}

	// Check which versions are actually installed
	installedVersions := make(map[string]bool)
	for _, version := range sortedVersions {
		_, err := installer.FindBinaryPath(owner, repo, version)
		installedVersions[version] = err == nil
	}

	// Define styles with TTY-aware dark/light mode detection
	profile := termenv.ColorProfile()
	var installedStyle, notInstalledStyle lipgloss.Style

	if profile == termenv.ANSI256 || profile == termenv.TrueColor {
		// Dark background - use grayscale
		installedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")) // Bright white
		notInstalledStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")) // Dim gray
	} else {
		// Light background or no color support - use basic styling
		installedStyle = lipgloss.NewStyle().
			Bold(true)
		notInstalledStyle = lipgloss.NewStyle()
	}

	// Display the results
	for _, version := range sortedVersions {
		isInstalled := installedVersions[version]
		isDefault := version == defaultVersion
		indicator := " "
		if isDefault {
			indicator = checkMark.String()
		}

		// Apply styling based on installation status
		if isInstalled {
			fmt.Printf("%s %s\n", indicator, installedStyle.Render(version))
		} else {
			fmt.Printf("%s %s\n", indicator, notInstalledStyle.Render(version))
		}
	}

	return nil
}

// sortVersionsSemver sorts versions in semantic version order
func sortVersionsSemver(versions []string) ([]string, error) {
	// Create a slice of semver versions
	var semverVersions []*semver.Version
	var nonSemverVersions []string

	for _, version := range versions {
		// Handle special versions like "latest", "system", etc.
		if isSpecialVersion(version) {
			nonSemverVersions = append(nonSemverVersions, version)
			continue
		}

		// Try to parse as semver
		v, err := semver.NewVersion(version)
		if err != nil {
			// If it's not a valid semver, treat it as a special version
			nonSemverVersions = append(nonSemverVersions, version)
			continue
		}
		semverVersions = append(semverVersions, v)
	}

	// Sort semver versions
	sort.Sort(semver.Collection(semverVersions))

	// Convert back to strings
	var result []string
	for _, v := range semverVersions {
		result = append(result, v.Original())
	}

	// Add non-semver versions at the end, sorted alphabetically
	sort.Strings(nonSemverVersions)
	result = append(result, nonSemverVersions...)

	return result, nil
}

// isSpecialVersion checks if a version string is a special version (not semver)
func isSpecialVersion(version string) bool {
	specialVersions := []string{
		"latest", "system", "current", "stable", "nightly", "dev", "master", "main",
		"head", "tip", "edge", "beta", "alpha", "rc", "pre", "snapshot",
	}

	versionLower := strings.ToLower(version)
	for _, special := range specialVersions {
		if versionLower == special || strings.HasPrefix(versionLower, special) {
			return true
		}
	}

	return false
}
