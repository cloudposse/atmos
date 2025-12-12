package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	versionLogKey = "version" // Log key for version information.
)

// URLRegistry fetches tool metadata from a custom URL.
// Supports two modes:
// 1. Single index file (source ends with .yaml/.yml) - all packages in one file.
// 2. Directory structure (source ends with / or no extension) - per-tool registry files.
type URLRegistry struct {
	baseURL    string
	client     httpClient.Client
	cache      map[string]*Tool // Simple in-memory cache for per-tool lookups.
	indexCache map[string]*Tool // Cache for index-based lookups.
	isIndexURL bool             // True if baseURL points to a single index file.
}

// NewURLRegistry creates a new URL-based registry.
func NewURLRegistry(baseURL string) *URLRegistry {
	defer perf.Track(nil, "registry.NewURLRegistry")()

	// Detect if source is a single index file or directory.
	isIndexFile := strings.HasSuffix(baseURL, ".yaml") || strings.HasSuffix(baseURL, ".yml")

	reg := &URLRegistry{
		baseURL:    baseURL,
		client:     httpClient.NewDefaultClient(httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv())),
		cache:      make(map[string]*Tool),
		indexCache: make(map[string]*Tool),
		isIndexURL: isIndexFile,
	}

	// If it's an index file, fetch and cache all packages upfront.
	if isIndexFile {
		if err := reg.loadIndex(); err != nil {
			// Log error but don't fail - will fall back to per-tool lookup.
			// Note: We can't return error from constructor, so we just mark it as non-index.
			reg.isIndexURL = false
		}
	}

	return reg
}

// GetTool fetches tool metadata from the custom URL.
func (ur *URLRegistry) GetTool(owner, repo string) (*Tool, error) {
	defer perf.Track(nil, "registry.URLRegistry.GetTool")()

	cacheKey := fmt.Sprintf("%s/%s", owner, repo)

	// If using index file, check index cache first.
	if ur.isIndexURL {
		if tool, ok := ur.indexCache[cacheKey]; ok {
			return tool, nil
		}
		return nil, fmt.Errorf("%w: %s/%s not found in registry index",
			ErrToolNotFound, owner, repo)
	}

	// For directory-based registries, check per-tool cache.
	if cached, ok := ur.cache[cacheKey]; ok {
		return cached, nil
	}

	// Try multiple path patterns for directory-based registries.
	baseURL := strings.TrimSuffix(ur.baseURL, "/")
	possibleURLs := []string{
		fmt.Sprintf("%s/%s/%s/registry.yaml", baseURL, owner, repo),
		fmt.Sprintf("%s/%s/registry.yaml", baseURL, repo),
	}

	for _, url := range possibleURLs {
		tool, err := ur.fetchFromURL(url)
		if err == nil {
			// Cache the result.
			ur.cache[cacheKey] = tool
			return tool, nil
		}
	}

	return nil, fmt.Errorf("%w: %s/%s not found in registry %s",
		ErrToolNotFound, owner, repo, ur.baseURL)
}

// GetToolWithVersion fetches tool metadata and resolves version-specific overrides.
func (ur *URLRegistry) GetToolWithVersion(owner, repo, version string) (*Tool, error) {
	defer perf.Track(nil, "registry.URLRegistry.GetToolWithVersion")()

	// Get base tool metadata.
	tool, err := ur.GetTool(owner, repo)
	if err != nil {
		return nil, err
	}

	// Set the version.
	tool.Version = version

	// Apply version overrides if present.
	if len(tool.VersionOverrides) > 0 {
		if err := applyVersionOverride(tool, version); err != nil {
			log.Warn("Failed to apply version override", "error", err, "owner", owner, "repo", repo, versionLogKey, version)
		}
	}

	return tool, nil
}

// GetLatestVersion is not implemented for URL registries.
// URL registries don't have version information; they only provide tool metadata.
func (ur *URLRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "registry.URLRegistry.GetLatestVersion")()

	return "", fmt.Errorf("%w: URL registries do not support version queries",
		ErrNoVersionsFound)
}

// LoadLocalConfig is a no-op for URL registries.
func (ur *URLRegistry) LoadLocalConfig(configPath string) error {
	defer perf.Track(nil, "registry.URLRegistry.LoadLocalConfig")()

	// URL registries don't use local config.
	return nil
}

// fetchFromURL fetches and parses a registry file from a URL.
func (ur *URLRegistry) fetchFromURL(url string) (*Tool, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request for %s: %w", ErrHTTPRequest, url, err)
	}

	resp, err := ur.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch %s: %w", ErrHTTPRequest, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrHTTPRequest, resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the registry file (Aqua format).
	var registryFile struct {
		Packages []AquaPackage `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &registryFile); err != nil {
		return nil, fmt.Errorf("%w: failed to parse registry YAML: %w", ErrRegistryParse, err)
	}

	if len(registryFile.Packages) == 0 {
		return nil, fmt.Errorf("%w: no packages found in registry file", ErrNoPackagesInRegistry)
	}

	// Convert first package to Tool.
	pkg := registryFile.Packages[0]
	tool := &Tool{
		Name:             pkg.RepoName,
		Type:             pkg.Type,
		RepoOwner:        pkg.RepoOwner,
		RepoName:         pkg.RepoName,
		URL:              pkg.URL,
		Format:           pkg.Format,
		VersionOverrides: pkg.VersionOverrides,
	}

	// Handle github_release vs http type.
	if pkg.Type == "github_release" {
		// Use URL as Asset for github_release.
		tool.Asset = pkg.URL
	}

	if pkg.BinaryName != "" {
		tool.Name = pkg.BinaryName
		tool.BinaryName = pkg.BinaryName
	}

	return tool, nil
}

// loadIndex fetches and caches all packages from an index file.
func (ur *URLRegistry) loadIndex() error {
	defer perf.Track(nil, "registry.URLRegistry.loadIndex")()

	req, err := http.NewRequest("GET", ur.baseURL, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to create request for %s: %w", ErrHTTPRequest, ur.baseURL, err)
	}

	resp, err := ur.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: failed to fetch index %s: %w", ErrHTTPRequest, ur.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d: %s", ErrHTTPRequest, resp.StatusCode, ur.baseURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}

	// Parse the index file (Aqua format with multiple packages).
	var indexFile struct {
		Packages []AquaPackage `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &indexFile); err != nil {
		return fmt.Errorf("%w: failed to parse index YAML: %w", ErrRegistryParse, err)
	}

	if len(indexFile.Packages) == 0 {
		return fmt.Errorf("%w: no packages found in index file", ErrNoPackagesInRegistry)
	}

	// Cache all packages from the index.
	for i := range indexFile.Packages {
		pkg := &indexFile.Packages[i]
		tool := &Tool{
			Name:             pkg.RepoName,
			Type:             pkg.Type,
			RepoOwner:        pkg.RepoOwner,
			RepoName:         pkg.RepoName,
			URL:              pkg.URL,
			Format:           pkg.Format,
			VersionOverrides: pkg.VersionOverrides,
		}

		// Handle github_release vs http type.
		if pkg.Type == "github_release" {
			tool.Asset = pkg.URL
		}

		if pkg.BinaryName != "" {
			tool.Name = pkg.BinaryName
			tool.BinaryName = pkg.BinaryName
		}

		// Cache using owner/repo format.
		cacheKey := fmt.Sprintf("%s/%s", pkg.RepoOwner, pkg.RepoName)
		ur.indexCache[cacheKey] = tool
	}

	return nil
}

// Search searches tools in the URL registry.
// URL registries don't support full search, so this returns empty results.
func (ur *URLRegistry) Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error) {
	defer perf.Track(nil, "registry.URLRegistry.Search")()

	// URL registries don't support search, return empty.
	return []*Tool{}, nil
}

// ListAll lists all tools in the URL registry.
// URL registries don't support listing, so this returns empty results.
func (ur *URLRegistry) ListAll(ctx context.Context, opts ...ListOption) ([]*Tool, error) {
	defer perf.Track(nil, "registry.URLRegistry.ListAll")()

	// URL registries don't support full listing, return empty.
	return []*Tool{}, nil
}

// GetMetadata returns metadata about the URL registry.
func (ur *URLRegistry) GetMetadata(ctx context.Context) (*RegistryMetadata, error) {
	defer perf.Track(nil, "registry.URLRegistry.GetMetadata")()

	return &RegistryMetadata{
		Name:      "url-registry",
		Type:      "aqua",
		Source:    ur.baseURL,
		Priority:  0,
		ToolCount: 0, // Unknown for URL registries
	}, nil
}

// applyVersionOverride applies version-specific overrides to the tool.
// It evaluates version constraints and applies the first matching override.
// Aqua uses expressions like:
//   - `Version == "v0.0.1"` - Exact version match
//   - `semver("<= 0.0.16")` - Semver constraint
//   - `"true"` - Catch-all (matches any version)
func applyVersionOverride(tool *Tool, version string) error {
	defer perf.Track(nil, "registry.applyVersionOverride")()

	for i := range tool.VersionOverrides {
		override := &tool.VersionOverrides[i]
		matches, err := evaluateVersionConstraint(override.VersionConstraint, version)
		if err != nil {
			log.Debug("Failed to evaluate version constraint", "constraint", override.VersionConstraint, versionLogKey, version, "error", err)
			continue
		}

		if matches {
			// Apply the override fields to the tool.
			if override.Asset != "" {
				tool.Asset = override.Asset
			}
			if override.Format != "" {
				tool.Format = override.Format
			}
			if len(override.Files) > 0 {
				tool.Files = override.Files
			}
			if len(override.Replacements) > 0 {
				tool.Replacements = override.Replacements
			}

			log.Debug("Applied version override", versionLogKey, version, "constraint", override.VersionConstraint, "asset", tool.Asset, "format", tool.Format)
			return nil
		}
	}

	// No matching override found - this is not an error.
	log.Debug("No matching version override", versionLogKey, version, "overrides_count", len(tool.VersionOverrides))
	return nil
}

// evaluateVersionConstraint evaluates an Aqua version constraint expression.
// Supports:
//   - `"true"` - Always matches
//   - `"false"` - Never matches
//   - `Version == "v1.2.3"` - Exact version match
//   - `semver(">= 1.2.3")` - Semver constraint
func evaluateVersionConstraint(constraint, version string) (bool, error) {
	defer perf.Track(nil, "registry.evaluateVersionConstraint")()

	// Trim whitespace.
	constraint = strings.TrimSpace(constraint)

	// Handle literal true/false.
	if constraint == "true" || constraint == `"true"` {
		return true, nil
	}
	if constraint == "false" || constraint == `"false"` {
		return false, nil
	}

	// Handle exact version match: Version == "v1.2.3"
	if strings.HasPrefix(constraint, "Version ==") {
		expectedVersion := strings.TrimSpace(strings.TrimPrefix(constraint, "Version =="))
		expectedVersion = strings.Trim(expectedVersion, `"`)
		return version == expectedVersion, nil
	}

	// Handle semver constraint: semver(">= 1.2.3")
	if strings.HasPrefix(constraint, "semver(") && strings.HasSuffix(constraint, ")") {
		semverConstraint := strings.TrimPrefix(constraint, "semver(")
		semverConstraint = strings.TrimSuffix(semverConstraint, ")")
		semverConstraint = strings.Trim(semverConstraint, `"`)

		// Parse the version (handle both "v1.2.3" and "1.2.3").
		v, err := semver.NewVersion(version)
		if err != nil {
			return false, fmt.Errorf("invalid version %q: %w", version, err)
		}

		// Parse the constraint.
		c, err := semver.NewConstraint(semverConstraint)
		if err != nil {
			return false, fmt.Errorf("invalid semver constraint %q: %w", semverConstraint, err)
		}

		return c.Check(v), nil
	}

	return false, fmt.Errorf("%w: %q", errUtils.ErrUnsupportedVersionConstraint, constraint)
}
