package aqua

import (
	"context"
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

	"github.com/Masterminds/semver/v3"
	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
	"github.com/cloudposse/atmos/toolchain/registry"
	"github.com/cloudposse/atmos/toolchain/registry/cache"
)

const (
	versionPrefix               = "v"
	defaultFileWritePermissions = 0o644
	defaultMkdirPermissions     = 0o755
)

// init registers the Aqua registry as the default registry.
func init() {
	registry.RegisterDefaultRegistry(func() registry.ToolRegistry {
		return NewAquaRegistry()
	})
}

// AquaRegistry represents the Aqua registry structure.
type AquaRegistry struct {
	client        httpClient.Client
	cache         *RegistryCache
	cacheStore    cache.Store
	githubBaseURL string
}

// RegistryCache handles caching of registry files.
type RegistryCache struct {
	baseDir string
}

// scoredTool represents a tool with its relevance score.
type scoredTool struct {
	tool  *registry.Tool
	score int
}

// RegistryOption is a functional option for configuring AquaRegistry.
type RegistryOption func(*AquaRegistry)

// WithGitHubBaseURL sets the GitHub API base URL (primarily for testing).
func WithGitHubBaseURL(url string) RegistryOption {
	return func(ar *AquaRegistry) {
		ar.githubBaseURL = url
	}
}

// NewAquaRegistry creates a new Aqua registry client.
func NewAquaRegistry(opts ...RegistryOption) *AquaRegistry {
	defer perf.Track(nil, "aqua.NewAquaRegistry")()

	// Use XDG-compliant cache directory.
	// Falls back to ~/.cache/atmos/toolchain on most systems.
	cacheBaseDir, err := xdg.GetXDGCacheDir("toolchain", 0o755)
	if err != nil {
		// Fallback to temp dir if XDG fails.
		log.Debug("Failed to get XDG cache dir, using temp", "error", err)
		cacheBaseDir = filepath.Join(os.TempDir(), "atmos-toolchain-cache")
	}

	ar := &AquaRegistry{
		client: httpClient.NewDefaultClient(
			httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
		),
		cache: &RegistryCache{
			baseDir: filepath.Join(cacheBaseDir, "registry"),
		},
		cacheStore:    cache.NewFileStore(cacheBaseDir),
		githubBaseURL: "https://api.github.com", // default
	}

	// Apply options.
	for _, opt := range opts {
		opt(ar)
	}

	return ar
}

// get performs an HTTP GET request and returns the response.
// This is a helper method to adapt the pkg/http Client interface.
func (ar *AquaRegistry) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %w", registry.ErrHTTPRequest, err)
	}

	resp, err := ar.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// LoadLocalConfig is deprecated and no-op for compatibility.
func (ar *AquaRegistry) LoadLocalConfig(configPath string) error {
	defer perf.Track(nil, "aqua.AquaRegistry.LoadLocalConfig")()

	// No-op for backward compatibility.
	return nil
}

// GetTool fetches tool metadata from the Aqua registry.
func (ar *AquaRegistry) GetTool(owner, repo string) (*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetTool")()

	// Fall back to remote registry
	// Try multiple registry sources
	registries := []string{
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/refs/heads/main/pkgs",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/kubernetes/kubernetes",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/hashicorp",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu",
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
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

	return nil, fmt.Errorf("%w: %s/%s not found in any registry", registry.ErrToolNotFound, owner, repo)
}

// GetToolWithVersion fetches tool metadata and resolves version-specific overrides.
func (ar *AquaRegistry) GetToolWithVersion(owner, repo, version string) (*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetToolWithVersion")()

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
func (ar *AquaRegistry) resolveVersionOverrides(owner, repo, version string) (*registry.Tool, error) {
	// Fetch the registry file again to get version overrides
	registryURL := fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", owner, repo)

	resp, err := ar.get(registryURL)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch registry file: %w", registry.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", registry.ErrHTTPRequest, resp.StatusCode, registryURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read registry file: %w", registry.ErrHTTPRequest, err)
	}

	// Parse the full registry file with version overrides
	var registryFile struct {
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

	if err := yaml.Unmarshal(data, &registryFile); err != nil {
		return nil, fmt.Errorf("%w: failed to parse registry YAML: %w", registry.ErrRegistryParse, err)
	}

	if len(registryFile.Packages) == 0 {
		return nil, fmt.Errorf("%w: no packages found in registry file", registry.ErrNoPackagesInRegistry)
	}

	pkgDef := registryFile.Packages[0]

	// Create base tool
	tool := &registry.Tool{
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

// defaultMkdirPermissions moved to constants block

// fetchFromRegistry fetches tool metadata from a specific registry.
func (ar *AquaRegistry) fetchFromRegistry(registryURL, owner, repo string) (*registry.Tool, error) {
	// Create cache directory
	if err := os.MkdirAll(ar.cache.baseDir, defaultMkdirPermissions); err != nil {
		return nil, fmt.Errorf("%w: failed to create cache directory: %w", registry.ErrFileOperation, err)
	}

	// Try different possible file paths for Aqua registry structure
	possiblePaths := []string{
		fmt.Sprintf("%s/%s/%s/registry.yaml", registryURL, owner, repo),
		fmt.Sprintf("%s/%s/registry.yaml", registryURL, repo),
	}

	for _, url := range possiblePaths {
		tool, err := ar.fetchRegistryFile(url)
		if err == nil {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("%w: tool not found in registry", registry.ErrToolNotFound)
}

// fetchRegistryFile fetches and parses a registry.yaml file.
func (ar *AquaRegistry) fetchRegistryFile(url string) (*registry.Tool, error) {
	// Create cache key
	cacheKey := strings.ReplaceAll(url, "/", "_")
	cacheKey = strings.ReplaceAll(cacheKey, ":", "_")
	cacheFile := filepath.Join(ar.cache.baseDir, cacheKey+".yaml")

	// Check cache first
	if data, err := os.ReadFile(cacheFile); err == nil {
		return ar.parseRegistryFile(data)
	}

	// Fetch from remote
	resp, err := ar.get(url)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch %s: %w", registry.ErrHTTPRequest, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", registry.ErrHTTPRequest, resp.StatusCode, url)
	}

	// Read response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %w", registry.ErrHTTPRequest, err)
	}

	// Cache the response
	if err := os.WriteFile(cacheFile, data, defaultFileWritePermissions); err != nil {
		// Log but don't fail
		log.Debug("Failed to cache registry file", "error", err)
	}

	return ar.parseRegistryFile(data)
}

// parseRegistryFile parses Aqua registry YAML data.
func (ar *AquaRegistry) parseRegistryFile(data []byte) (*registry.Tool, error) {
	// Try AquaRegistryFile (packages)
	var aquaRegistry registry.AquaRegistryFile
	if err := yaml.Unmarshal(data, &aquaRegistry); err == nil && len(aquaRegistry.Packages) > 0 {
		pkg := aquaRegistry.Packages[0]
		// Convert AquaPackage to Tool
		tool := &registry.Tool{
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
	var toolRegistry registry.ToolRegistryFile
	if err := yaml.Unmarshal(data, &toolRegistry); err == nil && len(toolRegistry.Tools) > 0 {
		return &toolRegistry.Tools[0], nil
	}

	return nil, fmt.Errorf("%w: no tools or packages found in registry file", registry.ErrNoPackagesInRegistry)
}

// BuildAssetURL constructs the download URL for a tool version.
func (ar *AquaRegistry) BuildAssetURL(tool *registry.Tool, version string) (string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.BuildAssetURL")()

	if tool.Asset == "" {
		return "", fmt.Errorf("%w: no asset template defined for tool", registry.ErrNoAssetTemplate)
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
			return strings.TrimPrefix(s, versionPrefix)
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
		return "", fmt.Errorf("%w: failed to parse asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	var assetName strings.Builder
	if err := tmpl.Execute(&assetName, data); err != nil {
		return "", fmt.Errorf("%w: failed to execute asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	// For http type tools, the URL is already complete
	if tool.Type == "http" {
		return assetName.String(), nil
	}

	// For github_release type, construct GitHub release URL
	// Ensure version has v prefix for GitHub releases
	releaseVersion := version
	if !strings.HasPrefix(releaseVersion, versionPrefix) {
		releaseVersion = versionPrefix + releaseVersion
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, releaseVersion, assetName.String())

	return url, nil
}

// GetLatestVersion fetches the latest non-prerelease version from GitHub releases.
func (ar *AquaRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetLatestVersion")()

	// GitHub API endpoint for releases
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases", ar.githubBaseURL, owner, repo)

	resp, err := ar.get(apiURL)
	if err != nil {
		return "", fmt.Errorf("%w: failed to fetch releases from GitHub: %w", registry.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: GitHub API returned status %d", registry.ErrHTTPRequest, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read response body: %w", registry.ErrHTTPRequest, err)
	}

	// Parse the JSON response to find the latest non-prerelease version
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("%w: failed to parse releases JSON: %w", registry.ErrRegistryParse, err)
	}

	// Find the first non-prerelease release
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, versionPrefix)
			return version, nil
		}
	}

	return "", fmt.Errorf("%w: no non-prerelease versions found for %s/%s", registry.ErrNoVersionsFound, owner, repo)
}

// GetAvailableVersions fetches all available versions from GitHub releases.
func (ar *AquaRegistry) GetAvailableVersions(owner, repo string) ([]string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetAvailableVersions")()

	// GitHub API endpoint for releases
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases", ar.githubBaseURL, owner, repo)

	resp, err := ar.get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch releases from GitHub: %w", registry.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: GitHub API returned status %d", registry.ErrHTTPRequest, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response body: %w", registry.ErrHTTPRequest, err)
	}

	// Parse the JSON response
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("%w: failed to parse releases JSON: %w", registry.ErrRegistryParse, err)
	}

	// Extract all non-prerelease versions
	var versions []string
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, versionPrefix)
			versions = append(versions, version)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: no non-prerelease versions found for %s/%s", registry.ErrNoVersionsFound, owner, repo)
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

// Search searches for tools matching the query string.
// The query is matched against tool owner, repo, and description.
func (ar *AquaRegistry) Search(ctx context.Context, query string, opts ...registry.SearchOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.Search")()

	// Apply search options.
	config := &registry.SearchConfig{
		Limit: 20, // default
	}
	for _, opt := range opts {
		opt(config)
	}

	// Get all tools from registry.
	allTools, err := ar.ListAll(ctx, registry.WithListLimit(0)) // 0 = no limit
	if err != nil {
		return nil, err
	}

	// Filter and score results.
	var results []scoredTool
	queryLower := strings.ToLower(query)

	for _, tool := range allTools {
		score := ar.calculateRelevanceScore(tool, queryLower)
		if score > 0 {
			results = append(results, scoredTool{tool: tool, score: score})
		}
	}

	// Sort by score (highest first), then alphabetically.
	sortResults(results)

	// Apply offset and limit.
	start := config.Offset
	if start > len(results) {
		start = len(results)
	}

	end := start + config.Limit
	if config.Limit == 0 || end > len(results) {
		end = len(results)
	}

	filtered := make([]*registry.Tool, 0, end-start)
	for i := start; i < end; i++ {
		filtered = append(filtered, results[i].tool)
	}

	return filtered, nil
}

// calculateRelevanceScore scores a tool based on query match.
// Scoring algorithm:
// - Exact match on repo name: 100
// - Prefix match on repo name: 70
// - Contains match on repo name: 50
// - Prefix match on owner: 40
// - Contains match on owner: 20.
func (ar *AquaRegistry) calculateRelevanceScore(tool *registry.Tool, queryLower string) int {
	repoLower := strings.ToLower(tool.RepoName)
	ownerLower := strings.ToLower(tool.RepoOwner)

	// Exact repo match.
	if repoLower == queryLower {
		return 100
	}

	score := 0

	// Prefix match on repo.
	if strings.HasPrefix(repoLower, queryLower) {
		score += 70
	} else if strings.Contains(repoLower, queryLower) {
		// Contains match on repo.
		score += 50
	}

	// Prefix match on owner.
	if strings.HasPrefix(ownerLower, queryLower) {
		score += 40
	} else if strings.Contains(ownerLower, queryLower) {
		// Contains match on owner.
		score += 20
	}

	return score
}

// sortResults sorts scored tools by score (descending) then alphabetically.
func sortResults(results []scoredTool) {
	// Simple bubble sort for small result sets.
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			// Sort by score descending.
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			} else if results[j].score == results[i].score {
				// For equal scores, sort alphabetically by repo name.
				if results[j].tool.RepoName < results[i].tool.RepoName {
					results[i], results[j] = results[j], results[i]
				}
			}
		}
	}
}

// ListAll returns all tools available in the Aqua registry.
func (ar *AquaRegistry) ListAll(ctx context.Context, opts ...registry.ListOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.ListAll")()

	// Apply list options.
	config := &registry.ListConfig{
		Limit: 50,     // default
		Sort:  "name", // default
	}
	for _, opt := range opts {
		opt(config)
	}

	// Fetch the main registry index.
	// For now, we'll fetch a known list of popular tools.
	// In V2, we should fetch the complete registry index.
	tools, err := ar.fetchRegistryIndex(ctx)
	if err != nil {
		return nil, err
	}

	// Sort results.
	if config.Sort == "name" {
		sortToolsByName(tools)
	}

	// Apply pagination.
	start := config.Offset
	if start > len(tools) {
		start = len(tools)
	}

	end := start + config.Limit
	if config.Limit == 0 || end > len(tools) {
		end = len(tools)
	}

	return tools[start:end], nil
}

// fetchRegistryIndex fetches the complete registry index from aqua-registry.
func (ar *AquaRegistry) fetchRegistryIndex(ctx context.Context) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.fetchRegistryIndex")()

	const cacheKey = "aqua-registry-index"
	const cacheTTL = 24 * time.Hour

	// Try to get from cache first.
	if cachedData, err := ar.cacheStore.Get(ctx, cacheKey); err == nil {
		// Cache hit - parse and return.
		tools, err := ar.parseIndexYAML(cachedData)
		if err == nil {
			log.Debug("Using cached registry index", "tool_count", len(tools))
			return tools, nil
		}
		// If parse fails, continue to fetch fresh data.
		log.Debug("Failed to parse cached index, fetching fresh", "error", err)
	}

	// Cache miss or expired - fetch from GitHub.
	indexURL := "https://raw.githubusercontent.com/aquaproj/aqua-registry/main/registry.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %w", registry.ErrHTTPRequest, err)
	}

	resp, err := ar.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch registry index: %w", registry.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: failed to fetch registry index (HTTP %d)", registry.ErrHTTPRequest, resp.StatusCode)
	}

	// Read response body.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read registry index: %w", registry.ErrHTTPRequest, err)
	}

	// Parse the index.
	tools, err := ar.parseIndexYAML(data)
	if err != nil {
		return nil, err
	}

	// Store in cache for next time.
	if err := ar.cacheStore.Set(ctx, cacheKey, data, cacheTTL); err != nil {
		log.Debug("Failed to cache registry index", "error", err)
		// Non-fatal - continue with fetched data.
	}

	log.Debug("Fetched registry index", "tool_count", len(tools))
	return tools, nil
}

// parseIndexYAML parses the aqua-registry registry.yaml format.
func (ar *AquaRegistry) parseIndexYAML(data []byte) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.parseIndexYAML")()

	var index struct {
		Packages []struct {
			Type      string `yaml:"type"`
			RepoOwner string `yaml:"repo_owner"`
			RepoName  string `yaml:"repo_name"`
		} `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("%w: failed to parse registry index: %w", registry.ErrRegistryParse, err)
	}

	// Convert to Tool objects.
	tools := make([]*registry.Tool, 0, len(index.Packages))
	for _, pkg := range index.Packages {
		tools = append(tools, &registry.Tool{
			RepoOwner: pkg.RepoOwner,
			RepoName:  pkg.RepoName,
			Type:      pkg.Type,
			Registry:  "aqua-public",
		})
	}

	return tools, nil
}

// sortToolsByName sorts tools alphabetically by repo name.
func sortToolsByName(tools []*registry.Tool) {
	for i := 0; i < len(tools); i++ {
		for j := i + 1; j < len(tools); j++ {
			if tools[j].RepoName < tools[i].RepoName {
				tools[i], tools[j] = tools[j], tools[i]
			}
		}
	}
}

// GetMetadata returns metadata about the Aqua registry.
func (ar *AquaRegistry) GetMetadata(ctx context.Context) (*registry.RegistryMetadata, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetMetadata")()

	// Get tool count by listing all tools.
	tools, err := ar.ListAll(ctx, registry.WithListLimit(0))
	if err != nil {
		return nil, err
	}

	return &registry.RegistryMetadata{
		Name:        "aqua-public",
		Type:        "aqua",
		Source:      "https://github.com/aquaproj/aqua-registry",
		Priority:    10, // Default priority
		ToolCount:   len(tools),
		LastUpdated: time.Now(), // TODO: Fetch actual last updated from GitHub API
	}, nil
}
