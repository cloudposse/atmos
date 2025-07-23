package main

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
	"time"

	"gopkg.in/yaml.v3"
)

// AquaRegistry represents the Aqua registry structure
type AquaRegistry struct {
	client *http.Client
	cache  *RegistryCache
	local  *LocalConfigManager
}

// RegistryCache handles caching of registry files
type RegistryCache struct {
	baseDir string
}

// NewAquaRegistry creates a new Aqua registry client
func NewAquaRegistry() *AquaRegistry {
	return &AquaRegistry{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: &RegistryCache{
			baseDir: filepath.Join(os.TempDir(), "installer-registry-cache"),
		},
		local: NewLocalConfigManager(),
	}
}

// LoadLocalConfig loads the local configuration
func (ar *AquaRegistry) LoadLocalConfig(configPath string) error {
	return ar.local.Load(configPath)
}

// GetPackage fetches package metadata from the Aqua registry
func (ar *AquaRegistry) GetPackage(owner, repo string) (*Package, error) {
	// Check local configuration first
	if localTool, exists := ar.local.GetTool(owner, repo); exists {
		Logger.Debug("Using local configuration", "owner", owner, "repo", repo)
		return ar.convertLocalToolToPackage(localTool, owner, repo), nil
	}

	// Fall back to remote registry
	// Try multiple registry sources
	registries := []string{
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/terraform",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm",
	}

	for _, registry := range registries {
		Logger.Debug("Trying registry", "registry", registry)
		pkg, err := ar.fetchFromRegistry(registry, owner, repo)
		if err == nil {
			Logger.Debug("Found package in registry", "registry", registry)
			return pkg, nil
		}
		Logger.Debug("Not found in registry", "registry", registry, "error", err)
	}

	return nil, fmt.Errorf("package %s/%s not found in any registry", owner, repo)
}

// GetPackageWithVersion fetches package metadata and resolves version-specific overrides
func (ar *AquaRegistry) GetPackageWithVersion(owner, repo, version string) (*Package, error) {
	pkg, err := ar.GetPackage(owner, repo)
	if err != nil {
		return nil, err
	}

	// If the package has version overrides, we need to fetch the full registry file
	// and resolve the correct asset template for this version
	if pkg.Type == "github_release" {
		return ar.resolveVersionOverrides(owner, repo, version)
	}

	return pkg, nil
}

// resolveVersionOverrides fetches the full registry file and resolves version-specific overrides
func (ar *AquaRegistry) resolveVersionOverrides(owner, repo, version string) (*Package, error) {
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

	// Create base package
	pkg := &Package{
		Name:      repo,
		Type:      pkgDef.Type,
		RepoOwner: pkgDef.RepoOwner,
		RepoName:  pkgDef.RepoName,
	}

	// Find the appropriate version override
	// For now, use the last override (usually the catch-all "true" constraint)
	if len(pkgDef.VersionOverrides) > 0 {
		override := pkgDef.VersionOverrides[len(pkgDef.VersionOverrides)-1]
		pkg.Asset = override.Asset
		// Store format for later use
		if override.Format != "" {
			pkg.Format = override.Format
		}
		// Extract binary name from files section
		if len(override.Files) > 0 {
			pkg.Name = override.Files[0].Name
		}
	} else {
		// Fallback to default template
		pkg.Asset = "{{.RepoName}}_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}"
	}

	return pkg, nil
}

// fetchFromRegistry fetches package metadata from a specific registry
func (ar *AquaRegistry) fetchFromRegistry(registryURL, owner, repo string) (*Package, error) {
	// Create cache directory
	if err := os.MkdirAll(ar.cache.baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Try different possible file paths for Aqua registry structure
	possiblePaths := []string{
		fmt.Sprintf("%s/%s/%s/registry.yaml", registryURL, owner, repo),
		fmt.Sprintf("%s/%s/registry.yaml", registryURL, repo),
	}

	for _, url := range possiblePaths {
		pkg, err := ar.fetchRegistryFile(url, owner, repo)
		if err == nil {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("package not found in registry")
}

// fetchRegistryFile fetches and parses a registry.yaml file
func (ar *AquaRegistry) fetchRegistryFile(url, owner, repo string) (*Package, error) {
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
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		// Log but don't fail
		Logger.Debug("Failed to cache registry file", "error", err)
	}

	return ar.parseRegistryFile(data, owner, repo)
}

// parseRegistryFile parses Aqua registry YAML data
func (ar *AquaRegistry) parseRegistryFile(data []byte, owner, repo string) (*Package, error) {
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
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if len(registry.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in registry file")
	}

	// Use the first package definition
	pkgDef := registry.Packages[0]

	// Create base package
	pkg := &Package{
		Name:      repo,
		Type:      pkgDef.Type,
		RepoOwner: pkgDef.RepoOwner,
		RepoName:  pkgDef.RepoName,
	}

	// Handle different package types
	switch pkgDef.Type {
	case "http":
		// Simple HTTP package with direct URL
		pkg.Asset = pkgDef.URL
	case "github_release":
		// GitHub release package - we'll need version to determine asset
		// For now, use a default asset template that can be overridden
		pkg.Asset = "{{.RepoName}}_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}"
		if len(pkgDef.VersionOverrides) > 0 {
			// Use the first version override as default
			pkg.Asset = pkgDef.VersionOverrides[0].Asset
		}
	default:
		return nil, fmt.Errorf("unsupported package type: %s", pkgDef.Type)
	}

	return pkg, nil
}

// BuildAssetURL constructs the download URL for a package version
func (ar *AquaRegistry) BuildAssetURL(pkg *Package, version string) (string, error) {
	if pkg.Asset == "" {
		return "", fmt.Errorf("no asset template defined for package")
	}

	// Determine format - use package format or default to zip
	format := "zip"
	if pkg.Format != "" {
		format = pkg.Format
	}

	// Create template data
	data := map[string]string{
		"Version":   version,
		"OS":        getOS(),
		"Arch":      getArch(),
		"RepoOwner": pkg.RepoOwner,
		"RepoName":  pkg.RepoName,
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
	tmpl, err := template.New("asset").Funcs(funcMap).Parse(pkg.Asset)
	if err != nil {
		return "", fmt.Errorf("failed to parse asset template: %w", err)
	}

	var assetName strings.Builder
	if err := tmpl.Execute(&assetName, data); err != nil {
		return "", fmt.Errorf("failed to execute asset template: %w", err)
	}

	// For http type packages, the URL is already complete
	if pkg.Type == "http" {
		return assetName.String(), nil
	}

	// For github_release type, construct GitHub release URL
	// Ensure version has v prefix for GitHub releases
	releaseVersion := version
	if !strings.HasPrefix(releaseVersion, "v") {
		releaseVersion = "v" + releaseVersion
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		pkg.RepoOwner, pkg.RepoName, releaseVersion, assetName.String())

	return url, nil
}

// convertLocalToolToPackage converts a local tool definition to a Package
func (ar *AquaRegistry) convertLocalToolToPackage(localTool *LocalTool, owner, repo string) *Package {
	pkg := &Package{
		Name:      repo,
		Type:      localTool.Type,
		RepoOwner: localTool.RepoOwner,
		RepoName:  localTool.RepoName,
		Asset:     localTool.Asset,
		Format:    localTool.Format,
	}

	// Set binary name if specified
	if localTool.BinaryName != "" {
		pkg.Name = localTool.BinaryName
	}

	// Handle URL for http type
	if localTool.Type == "http" {
		pkg.Asset = localTool.URL
	}

	return pkg
}

// GetLatestVersion fetches the latest non-prerelease version from GitHub releases
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

// getOS returns the current operating system
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

// getArch returns the current architecture
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
