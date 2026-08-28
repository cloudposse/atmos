package toolchain

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ToolVersionInfo is the --format=json output shape for a single resolved tool version
// (i.e. `atmos toolchain get <tool> --format=json`, without --all).
type ToolVersionInfo struct {
	Tool      string `json:"tool" yaml:"tool"`
	Version   string `json:"version" yaml:"version"`
	Installed bool   `json:"installed" yaml:"installed"`
}

// ToolVersionListEntry is one entry in ToolVersionList.Versions.
type ToolVersionListEntry struct {
	Version   string `json:"version" yaml:"version"`
	Installed bool   `json:"installed" yaml:"installed"`
	Default   bool   `json:"default" yaml:"default"`
}

// ToolVersionList is the --format=json output shape for `atmos toolchain get <tool> --all --format=json`.
type ToolVersionList struct {
	Tool           string                 `json:"tool" yaml:"tool"`
	DefaultVersion string                 `json:"default_version,omitempty" yaml:"default_version,omitempty"`
	Versions       []ToolVersionListEntry `json:"versions" yaml:"versions"`
}

// supportedListFormats are the values ListToolVersions accepts for format.
var supportedListFormats = []string{"table", "plain", "json"}

// ListToolVersions handles the logic for listing tool versions.
func ListToolVersions(showAll bool, limit int, toolName, format string) error {
	defer perf.Track(nil, "toolchain.ListToolVersions")()

	if !slices.Contains(supportedListFormats, format) {
		return fmt.Errorf("%w: %q (supported: %v)", errUtils.ErrInvalidFlagValue, format, supportedListFormats)
	}
	if format == "plain" && showAll {
		return errUtils.ErrToolchainPlainFormatWithAllFlag
	}

	filePath := GetToolVersionsFilePath()
	installer := NewInstaller()

	owner, repo, err := installer.ParseToolSpec(toolName)
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

	switch format {
	case "plain":
		return printVersionsPlain(defaultVersion)
	case "json":
		return printVersionsJSON(resolvedKey, showAll, versions, defaultVersion, installed)
	default:
		installedStyle, notInstalledStyle := selectStyles()
		printVersionsTable(versions, defaultVersion, installed, &installedStyle, &notInstalledStyle)
		return nil
	}
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
	// Use ui.GetColorProfile() instead of termenv.ColorProfile() to respect
	// atmos's terminal detection (handles Terminal.app 256-color limitation).
	profile := ui.GetColorProfile()
	if profile == termenv.ANSI256 || profile == termenv.TrueColor {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")), // white
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray
	}
	return lipgloss.NewStyle().Bold(true), lipgloss.NewStyle()
}

func printVersionsTable(versions []string, defaultVersion string, installed map[string]bool, installedStyle, notInstalledStyle *lipgloss.Style) {
	for _, v := range versions {
		indicator := " "
		if v == defaultVersion {
			indicator = theme.Styles.Checkmark.Render()
		}
		if installed[v] {
			ui.Writef("%s %s", indicator, installedStyle.Render(v))
		} else {
			ui.Writef("%s %s", indicator, notInstalledStyle.Render(v))
		}
	}
}

// printVersionsPlain writes just the bare resolved version string to the data channel (stdout),
// with no styling — intended for shell substitution in scripts/CI.
func printVersionsPlain(defaultVersion string) error {
	return data.Writeln(defaultVersion)
}

// printVersionsJSON writes structured tool-version data to the data channel (stdout).
func printVersionsJSON(resolvedKey string, showAll bool, versions []string, defaultVersion string, installed map[string]bool) error {
	if showAll {
		entries := make([]ToolVersionListEntry, 0, len(versions))
		for _, v := range versions {
			entries = append(entries, ToolVersionListEntry{
				Version:   v,
				Installed: installed[v],
				Default:   v == defaultVersion,
			})
		}
		return data.WriteJSON(ToolVersionList{
			Tool:           resolvedKey,
			DefaultVersion: defaultVersion,
			Versions:       entries,
		})
	}

	return data.WriteJSON(ToolVersionInfo{
		Tool:      resolvedKey,
		Version:   defaultVersion,
		Installed: installed[defaultVersion],
	})
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
