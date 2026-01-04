package toolchain

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// toolVersionInfo holds version information for a tool.
type toolVersionInfo struct {
	InstalledVersions  []string
	ConfiguredVersions []string
	DefaultVersion     string
	ResolvedVersion    string
}

// resolveToolVersions gets installed, configured, and default versions from .tool-versions file.
func resolveToolVersions(toolName, owner, repo string, installer *Installer) (toolVersionInfo, error) {
	info := toolVersionInfo{
		InstalledVersions:  []string{},
		ConfiguredVersions: []string{},
	}

	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		return info, err
	}

	versions, exists := toolVersions.Tools[toolName]
	if !exists || len(versions) == 0 {
		// Try to resolve version using LookupToolVersionOrLatest.
		result := LookupToolVersionOrLatest(toolName, toolVersions, installer.GetResolver())
		info.ResolvedVersion = result.Version
		return info, nil
	}

	// Track all configured versions.
	info.ConfiguredVersions = versions

	// Check which versions are actually installed (have binaries on disk).
	for _, v := range versions {
		if _, err := installer.FindBinaryPath(owner, repo, v); err == nil {
			info.InstalledVersions = append(info.InstalledVersions, v)
			// First installed version becomes default.
			if info.DefaultVersion == "" {
				info.DefaultVersion = v
			}
		}
	}

	// If no installed versions, but versions exist in config, use last configured as default.
	if info.DefaultVersion == "" && len(versions) > 0 {
		info.DefaultVersion = versions[len(versions)-1]
	}

	// Try to find a version using LookupToolVersionOrLatest.
	result := LookupToolVersionOrLatest(toolName, toolVersions, installer.GetResolver())
	info.ResolvedVersion = result.Version

	return info, nil
}

// resolveLatestVersion resolves "latest" to a concrete version number from the registry.
func resolveLatestVersion(version, owner, repo string) string {
	// If no version found or if it's still "latest", resolve to concrete latest version.
	if version == "" || version == "latest" {
		// Get the actual latest version from the registry.
		reg := NewAquaRegistry()
		latestVersion, err := reg.GetLatestVersion(owner, repo)
		if err == nil {
			return latestVersion
		}
		// If we can't get the latest version, fall back to "latest" literal.
		return "latest"
	}
	return version
}

// getRegistryName fetches the registry name from metadata.
func getRegistryName(ctx context.Context) string {
	reg := NewAquaRegistry()
	if meta, err := reg.GetMetadata(ctx); err == nil {
		return meta.Name
	}
	return ""
}

// getAvailableVersions fetches available versions from GitHub with a limit.
func getAvailableVersions(owner, repo string, limit int) []versionItem {
	// Try to get available versions with full metadata from GitHub (with spinner).
	availableVersions, err := fetchGitHubVersionsWithSpinner(owner, repo)
	if err != nil {
		// Log error but don't fail - just show no available versions.
		return []versionItem{}
	}

	// Show latest N versions with full metadata.
	if len(availableVersions) < limit {
		limit = len(availableVersions)
	}
	return availableVersions[:limit]
}

// displayFormattedOutput displays tool information in the requested format.
func displayFormattedOutput(outputFormat string, tool *registry.Tool, version string, installer *Installer, ctx *toolContext) error {
	if outputFormat == "yaml" {
		evaluatedYAML, err := getEvaluatedToolYAML(tool, version, installer)
		if err != nil {
			return fmt.Errorf("failed to get evaluated YAML: %w", err)
		}
		_ = data.Write(evaluatedYAML)
	} else {
		// Enhanced table format (default).
		displayToolInfo(ctx)
	}
	return nil
}
