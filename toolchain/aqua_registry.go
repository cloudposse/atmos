package toolchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
)

// AquaRegistry represents the Aqua registry structure.
type AquaRegistry struct {
	client *http.Client
	cache  *RegistryCache
	local  *LocalConfigManager
}

// RegistryCache handles caching of registry files.
type RegistryCache struct {
	baseDir string
}

// NewAquaRegistry creates a new Aqua registry client.
func NewAquaRegistry() *AquaRegistry {
	return &AquaRegistry{
		client: NewDefaultHTTPClient(),
		cache: &RegistryCache{
			baseDir: filepath.Join(os.TempDir(), "tools-cache"),
		},
		local: NewLocalConfigManager(),
	}
}

// LoadLocalConfig loads the local configuration.
func (ar *AquaRegistry) LoadLocalConfig(configPath string) error {
	return ar.local.Load(configPath)
}

// GetTool fetches tool metadata from the Aqua registry.
func (ar *AquaRegistry) GetTool(owner, repo string) (*Tool, error) {
	// Check local configuration first
	if localTool, exists := ar.local.GetTool(owner, repo); exists {
		log.Debug("Using local configuration", "owner", owner, "repo", repo)
		return ar.convertLocalToolToTool(localTool, owner, repo), nil
	}

	// Fall back to remote registry
	// Try multiple registry sources
	registries := []string{
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/refs/heads/main/pkgs",
	}

	for _, registry := range registries {
		log.Debug("Trying registry", "registry", registry)
		tool, err := ar.fetchFromRegistry(registry, owner, repo)
		if err == nil {
			log.Debug("Found tool in registry", "registry", registry)
			return tool, nil
		}
		log.Debug("Not found in registry", "registry", registry, "error", err)
	}

	return nil, fmt.Errorf("tool %s/%s not found in any registry", owner, repo)
}

// GetToolWithVersion fetches tool metadata and resolves version-specific overrides.
func (ar *AquaRegistry) GetToolWithVersion(owner, repo, version string) (*Tool, error) {
	tool, err := ar.GetTool(owner, repo)
	if err != nil {
		return nil, err
	}

	// If the tool has version overrides, we need to fetch the full registry file
	// and resolve the correct asset template for this version
	if tool.Type == "github_release" {
		return ar.resolveVersionOverrides(owner, repo, version)
	}

	return tool, nil
}

// resolveVersionOverrides fetches the full registry file and resolves version-specific overrides.
func (ar *AquaRegistry) resolveVersionOverrides(owner, repo, version string) (*Tool, error) {
	// Fetch the registry file again to get version overrides
	registryURL := fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", owner, repo)

	resp, err := ar.client.Get(registryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, registryURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry file: %w", err)
	}

	// Parse the full registry file with version overrides
	var registry struct {
		Packages []struct {
			Type             string `yaml:"type"`
			RepoOwner        string `yaml:"repo_owner"`
			RepoName         string `yaml:"repo_name"`
			URL              string `yaml:"url"`
			Description      string `yaml:"description"`
			VersionOverrides []struct {
				VersionConstraint string `yaml:"version_constraint"`
				Asset             string `yaml:"asset"`
				Format            string `yaml:"format"`
				Files             []struct {
					Name string `yaml:"name"`
				} `yaml:"files"`
			} `yaml:"version_overrides"`
		} `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry YAML: %w", err)
	}

	if len(registry.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in registry file")
	}

	pkgDef := registry.Packages[0]

	// Create base tool
	tool := &Tool{
		Name:      repo,
		Type:      pkgDef.Type,
		RepoOwner: pkgDef.RepoOwner,
		RepoName:  pkgDef.RepoName,
	}

	// Find the appropriate version override using semver constraint
	v, err := semver.NewVersion(version)
	if err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}

	selectedIdx := -1
	for i, override := range pkgDef.VersionOverrides {
		c, err := semver.NewConstraint(override.VersionConstraint)
		if err != nil {
			continue // skip invalid constraints
		}
		if c.Check(v) {
			selectedIdx = i
			break // use the first matching override
		}
	}

	if selectedIdx != -1 {
		override := pkgDef.VersionOverrides[selectedIdx]
		if override.Asset != "" {
			tool.Asset = override.Asset
		}
		if override.Format != "" {
			tool.Format = override.Format
		}
		if len(override.Files) > 0 {
			tool.Name = override.Files[0].Name
		}
	}

	return tool, nil
}

// fetchFromRegistry fetches tool metadata from a specific registry.
func (ar *AquaRegistry) fetchFromRegistry(registryURL, owner, repo string) (*Tool, error) {
	// Create cache directory
	if err := os.MkdirAll(ar.cache.baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Try different possible file paths for Aqua registry structure
	possiblePaths := []string{
		fmt.Sprintf("%s/%s/%s/registry.yaml", registryURL, owner, repo),
		fmt.Sprintf("%s/%s/registry.yaml", registryURL, repo),
	}

	for _, url := range possiblePaths {
		tool, err := ar.fetchRegistryFile(url, owner, repo)
		if err == nil {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("tool not found in registry")
}

// fetchRegistryFile fetches and parses a registry.yaml file.
func (ar *AquaRegistry) fetchRegistryFile(url, owner, repo string) (*Tool, error) {
	// Create cache key
	cacheKey := strings.ReplaceAll(url, "/", "_")
	cacheKey = strings.ReplaceAll(cacheKey, ":", "_")
	cacheFile := filepath.Join(ar.cache.baseDir, cacheKey+".yaml")

	// Check cache first
	if data, err := os.ReadFile(cacheFile); err == nil {
		return ar.parseRegistryFile(data, owner, repo)
	}

	// Fetch from remote
	resp, err := ar.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	// Read response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Cache the response
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		// Log but don't fail
		log.Debug("Failed to cache registry file", "error", err)
	}

	return ar.parseRegistryFile(data, owner, repo)
}

// parseRegistryFile parses Aqua registry YAML data.
func (ar *AquaRegistry) parseRegistryFile(data []byte, owner, repo string) (*Tool, error) {
	// Try AquaRegistryFile (packages)
	var aquaRegistry AquaRegistryFile
	if err := yaml.Unmarshal(data, &aquaRegistry); err == nil && len(aquaRegistry.Packages) > 0 {
		pkg := aquaRegistry.Packages[0]
		// Convert AquaPackage to Tool
		tool := &Tool{
			Name:       pkg.BinaryName,
			RepoOwner:  pkg.RepoOwner,
			RepoName:   pkg.RepoName,
			Asset:      pkg.URL,
			Format:     pkg.Format,
			Type:       pkg.Type,
			BinaryName: pkg.BinaryName,
		}
		if pkg.BinaryName == "" {
			tool.Name = pkg.RepoName
		}
		return tool, nil
	}

	// Fallback to ToolRegistry (tools)
	var toolRegistry ToolRegistry
	if err := yaml.Unmarshal(data, &toolRegistry); err == nil && len(toolRegistry.Tools) > 0 {
		return &toolRegistry.Tools[0], nil
	}

	return nil, fmt.Errorf("no tools or packages found in registry file")
}

// BuildAssetURL constructs the download URL for a tool version.
func (ar *AquaRegistry) BuildAssetURL(tool *Tool, version string) (string, error) {
	if tool.Asset == "" {
		return "", fmt.Errorf("no asset template defined for tool")
	}

	// Determine format - use tool format or default to zip
	format := "zip"
	if tool.Format != "" {
		format = tool.Format
	}

	// Create template data
	data := map[string]string{
		"Version":   version,
		"OS":        getOS(),
		"Arch":      getArch(),
		"RepoOwner": tool.RepoOwner,
		"RepoName":  tool.RepoName,
		"Format":    format,
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"trimV": func(s string) string {
			return strings.TrimPrefix(s, "v")
		},
		"trimPrefix": func(prefix, s string) string {
			return strings.TrimPrefix(s, prefix)
		},
		"trimSuffix": func(suffix, s string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
	}

	// Parse and execute template
	tmpl, err := template.New("asset").Funcs(funcMap).Parse(tool.Asset)
	if err != nil {
		return "", fmt.Errorf("failed to parse asset template: %w", err)
	}

	var assetName strings.Builder
	if err := tmpl.Execute(&assetName, data); err != nil {
		return "", fmt.Errorf("failed to execute asset template: %w", err)
	}

	// For http type tools, the URL is already complete
	if tool.Type == "http" {
		return assetName.String(), nil
	}

	// For github_release type, construct GitHub release URL
	// Ensure version has v prefix for GitHub releases
	releaseVersion := version
	if !strings.HasPrefix(releaseVersion, "v") {
		releaseVersion = "v" + releaseVersion
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, releaseVersion, assetName.String())

	return url, nil
}

// convertLocalToolToTool converts a local tool definition to a Tool.
func (ar *AquaRegistry) convertLocalToolToTool(localTool *LocalTool, owner, repo string) *Tool {
	tool := &Tool{
		Name:      repo,
		Type:      localTool.Type,
		RepoOwner: localTool.RepoOwner,
		RepoName:  localTool.RepoName,
		Asset:     localTool.URL,
		Format:    localTool.Format,
	}

	// Set binary name if specified
	if localTool.BinaryName != "" {
		tool.Name = localTool.BinaryName
	}

	// Handle URL for http type
	if localTool.Type == "http" {
		tool.Asset = localTool.URL
	}

	return tool
}

// GetLatestVersion fetches the latest non-prerelease version from GitHub releases.
func (ar *AquaRegistry) GetLatestVersion(owner, repo string) (string, error) {
	// GitHub API endpoint for releases
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)

	resp, err := ar.client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response to find the latest non-prerelease version
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	// Find the first non-prerelease release
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, "v")
			return version, nil
		}
	}

	return "", fmt.Errorf("no non-prerelease versions found for %s/%s", owner, repo)
}

// GetAvailableVersions fetches all available versions from GitHub releases.
func (ar *AquaRegistry) GetAvailableVersions(owner, repo string) ([]string, error) {
	// GitHub API endpoint for releases
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)

	resp, err := ar.client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	// Extract all non-prerelease versions
	var versions []string
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, "v")
			versions = append(versions, version)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no non-prerelease versions found for %s/%s", owner, repo)
	}

	return versions, nil
}

// getOS returns the current operating system.
func getOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// getArch returns the current architecture.
func getArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	default:
		return "amd64"
	}
}
