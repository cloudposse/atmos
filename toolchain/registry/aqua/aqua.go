package aqua

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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
	githubPerPage               = 100 // Maximum results per page.

	// Search constants.
	defaultSearchLimit      = 20
	defaultListLimit        = 50
	defaultVersionLimit     = 10
	defaultRegistryPriority = 10
	registryLogKey          = "registry"
	durationMetricKey       = "duration"

	// Search scoring weights.
	scoreExactRepoMatch     = 100
	scoreRepoPrefixMatch    = 70
	scoreRepoContainsMatch  = 50
	scoreOwnerPrefixMatch   = 40
	scoreOwnerContainsMatch = 20
)

// init registers the Aqua registry as the default registry.
func init() {
	registry.RegisterDefaultRegistry(func() registry.ToolRegistry {
		return NewAquaRegistry()
	})
}

// AquaRegistry represents the Aqua registry structure.
type AquaRegistry struct {
	client          httpClient.Client
	cache           *RegistryCache
	cacheStore      cache.Store
	githubBaseURL   string
	lastSearchTotal int // Total number of search results before pagination.
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
	defer perf.Track(nil, "aqua.WithGitHubBaseURL")()

	return func(ar *AquaRegistry) {
		ar.githubBaseURL = url
	}
}

// NewAquaRegistry creates a new Aqua registry client.
func NewAquaRegistry(opts ...RegistryOption) *AquaRegistry {
	defer perf.Track(nil, "aqua.NewAquaRegistry")()

	// Use XDG-compliant cache directory.
	// Falls back to ~/.cache/atmos/toolchain on most systems.
	cacheBaseDir, err := xdg.GetXDGCacheDir("toolchain", defaultMkdirPermissions)
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
		log.Debug("Trying registry", registryLogKey, registry)
		tool, err := ar.fetchFromRegistry(registry, owner, repo)
		if err == nil {
			log.Debug("Found tool in registry", registryLogKey, registry)
			return tool, nil
		}
		log.Debug("Not found in registry", registryLogKey, registry, "error", err)
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

// versionOverride holds version override data from Aqua registry.
type versionOverride struct {
	VersionConstraint string `yaml:"version_constraint"`
	Asset             string `yaml:"asset"`
	Format            string `yaml:"format"`
	Files             []struct {
		Name string `yaml:"name"`
	} `yaml:"files"`
}

// applyVersionOverride applies a version override to the tool.
func applyVersionOverride(tool *registry.Tool, override versionOverride, version string) {
	if override.Asset != "" {
		tool.Asset = override.Asset
	}
	if override.Format != "" {
		tool.Format = override.Format
	}
	if len(override.Files) > 0 {
		tool.Name = override.Files[0].Name
	}
	log.Debug("Applied version override", "version", version, "constraint", override.VersionConstraint, "asset", tool.Asset, "format", tool.Format)
}

// registryPackage holds package data from Aqua registry file.
type registryPackage struct {
	Type             string            `yaml:"type"`
	RepoOwner        string            `yaml:"repo_owner"`
	RepoName         string            `yaml:"repo_name"`
	URL              string            `yaml:"url"`
	Description      string            `yaml:"description"`
	VersionOverrides []versionOverride `yaml:"version_overrides"`
}

// resolveVersionOverrides fetches the full registry file and resolves version-specific overrides.
func (ar *AquaRegistry) resolveVersionOverrides(owner, repo, version string) (*registry.Tool, error) {
	registryURL := fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", owner, repo)

	pkgDef, err := ar.fetchRegistryPackage(registryURL)
	if err != nil {
		return nil, err
	}

	tool := &registry.Tool{
		Name:      repo,
		Type:      pkgDef.Type,
		RepoOwner: pkgDef.RepoOwner,
		RepoName:  pkgDef.RepoName,
	}

	selectedIdx := findMatchingOverride(pkgDef.VersionOverrides, version)
	if selectedIdx == -1 {
		log.Debug("No matching version override", "version", version, "overrides_count", len(pkgDef.VersionOverrides))
		return tool, nil
	}

	applyVersionOverride(tool, pkgDef.VersionOverrides[selectedIdx], version)
	return tool, nil
}

// fetchRegistryPackage fetches and parses a registry file, returning the first package.
func (ar *AquaRegistry) fetchRegistryPackage(registryURL string) (*registryPackage, error) {
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

	var registryFile struct {
		Packages []registryPackage `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &registryFile); err != nil {
		return nil, fmt.Errorf("%w: failed to parse registry YAML: %w", registry.ErrRegistryParse, err)
	}

	if len(registryFile.Packages) == 0 {
		return nil, fmt.Errorf("%w: no packages found in registry file", registry.ErrNoPackagesInRegistry)
	}

	return &registryFile.Packages[0], nil
}

// findMatchingOverride finds the first version override that matches the given version.
func findMatchingOverride(overrides []versionOverride, version string) int {
	for i, override := range overrides {
		matches, err := evaluateVersionConstraint(override.VersionConstraint, version)
		if err != nil {
			log.Debug("Failed to evaluate version constraint", "constraint", override.VersionConstraint, "version", version, "error", err)
			continue
		}
		if matches {
			return i
		}
	}
	return -1
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
		// Convert AquaPackage to Tool.
		tool := &registry.Tool{
			Name:          pkg.BinaryName,
			RepoOwner:     pkg.RepoOwner,
			RepoName:      pkg.RepoName,
			Asset:         pkg.URL,
			Format:        pkg.Format,
			Type:          pkg.Type,
			BinaryName:    pkg.BinaryName,
			VersionPrefix: pkg.VersionPrefix,
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

	releaseVersion, semVer := resolveVersionStrings(tool, version)
	data := buildAssetTemplateData(tool, releaseVersion, semVer)

	assetName, err := executeAssetTemplate(tool.Asset, data)
	if err != nil {
		return "", err
	}

	// For http type tools, the URL is already complete.
	if tool.Type == "http" {
		return assetName, nil
	}

	// For github_release type, construct GitHub release URL using the full tag.
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, releaseVersion, assetName), nil
}

// resolveVersionStrings determines the release version and semver based on tool config.
func resolveVersionStrings(tool *registry.Tool, version string) (releaseVersion, semVer string) {
	prefix := tool.VersionPrefix
	if prefix == "" {
		prefix = versionPrefix
	}

	releaseVersion = version
	if !strings.HasPrefix(releaseVersion, prefix) {
		releaseVersion = prefix + releaseVersion
	}

	semVer = strings.TrimPrefix(releaseVersion, prefix)
	return releaseVersion, semVer
}

// buildAssetTemplateData creates the template data map for asset URL rendering.
func buildAssetTemplateData(tool *registry.Tool, releaseVersion, semVer string) map[string]string {
	format := "zip"
	if tool.Format != "" {
		format = tool.Format
	}

	return map[string]string{
		"Version":   releaseVersion,
		"SemVer":    semVer,
		"OS":        getOS(),
		"Arch":      getArch(),
		"RepoOwner": tool.RepoOwner,
		"RepoName":  tool.RepoName,
		"Format":    format,
	}
}

// assetTemplateFuncs returns the template function map for asset URL templates.
func assetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
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
}

// executeAssetTemplate parses and executes the asset template.
func executeAssetTemplate(assetTemplate string, data map[string]string) (string, error) {
	tmpl, err := template.New("asset").Funcs(assetTemplateFuncs()).Parse(assetTemplate)
	if err != nil {
		return "", fmt.Errorf("%w: failed to parse asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	var assetName strings.Builder
	if err := tmpl.Execute(&assetName, data); err != nil {
		return "", fmt.Errorf("%w: failed to execute asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	return assetName.String(), nil
}
