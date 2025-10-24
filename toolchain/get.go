package toolchain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ListToolVersions handles the logic for listing tool versions.
func ListToolVersions(showAll bool, limit int, toolName string) error {
	defer perf.Track(nil, "toolchain.GetVersionForTool")()

	filePath := GetToolVersionsFilePath()
	installer := NewInstaller()

	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}
	resolvedKey := owner + "/" + repo

	versions, defaultVersion, err := getVersions(&versionOptions{showAll, limit, owner, repo, toolName, resolvedKey, filePath})
	if err != nil {
		return err
	}

	versions = dedupeAndSort(versions)
	installed := markInstalled(installer, owner, repo, versions)
	installedStyle, notInstalledStyle := selectStyles()

	printVersions(versions, defaultVersion, installed, installedStyle, notInstalledStyle)
	return nil
}

type versionOptions struct {
	ShowAll     bool
	Limit       int
	Owner       string
	Repo        string
	ToolName    string
	ResolvedKey string
	FilePath    string
}

func getVersions(opts *versionOptions) ([]string, string, error) {
	if opts.ShowAll {
		allVersions, err := fetchAllGitHubVersions(opts.Owner, opts.Repo, opts.Limit)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch versions from GitHub: %w", err)
		}
		defaultVersion := getDefaultFromFile(opts.FilePath, opts.ResolvedKey, opts.ToolName)
		return allVersions, defaultVersion, nil
	}

	toolVersions, err := LoadToolVersions(opts.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	fileVersions, exists := toolVersions.Tools[opts.ResolvedKey]
	if !exists {
		fileVersions, exists = toolVersions.Tools[opts.ToolName]
		if !exists {
			return nil, "", fmt.Errorf("%w: tool '%s' not found in %s", ErrToolNotFound, opts.ToolName, opts.FilePath)
		}
	}
	if len(fileVersions) == 0 {
		return nil, "", fmt.Errorf("%w: no versions configured for tool '%s' in %s", ErrNoVersionsFound, opts.ToolName, opts.FilePath)
	}
	return fileVersions, fileVersions[0], nil
}

func getDefaultFromFile(filePath, resolvedKey, toolName string) string {
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return ""
	}
	if configured, ok := toolVersions.Tools[resolvedKey]; ok && len(configured) > 0 {
		return configured[0]
	}
	if configured, ok := toolVersions.Tools[toolName]; ok && len(configured) > 0 {
		return configured[0]
	}
	return ""
}

func dedupeAndSort(versions []string) []string {
	seen := make(map[string]struct{})
	unique := []string{}
	for _, v := range versions {
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			unique = append(unique, v)
		}
	}
	return sortVersionsSemver(unique)
}

func markInstalled(installer *Installer, owner, repo string, versions []string) map[string]bool {
	installed := make(map[string]bool, len(versions))
	for _, v := range versions {
		_, err := installer.FindBinaryPath(owner, repo, v)
		installed[v] = err == nil
	}
	return installed
}

func selectStyles() (lipgloss.Style, lipgloss.Style) {
	profile := termenv.ColorProfile()
	if profile == termenv.ANSI256 || profile == termenv.TrueColor {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")), // white
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray
	}
	return lipgloss.NewStyle().Bold(true), lipgloss.NewStyle()
}

func printVersions(versions []string, defaultVersion string, installed map[string]bool, installedStyle, notInstalledStyle lipgloss.Style) {
	for _, v := range versions {
		indicator := " "
		if v == defaultVersion {
			indicator = checkMark.String()
		}
		if installed[v] {
			fmt.Printf("%s %s\n", indicator, installedStyle.Render(v))
		} else {
			fmt.Printf("%s %s\n", indicator, notInstalledStyle.Render(v))
		}
	}
}

// sortVersionsSemver sorts versions in semantic version order.
func sortVersionsSemver(versions []string) []string {
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

	return result
}

// isSpecialVersion checks if a version string is a special version (not semver).
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
