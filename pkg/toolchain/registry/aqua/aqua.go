package aqua

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"
	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/registry/cache"
	"github.com/cloudposse/atmos/pkg/xdg"
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
	versionLogKey           = "version"

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

	// Both github_release and http type tools can have version overrides
	// that need to be resolved (e.g., kubectl has version-specific URLs).
	if tool.Type == "github_release" || tool.Type == "http" {
		// Use the SourceURL from GetTool to fetch the same registry file
		// for version override resolution. This handles nested paths like
		// kubernetes/kubernetes/kubectl correctly.
		return ar.resolveVersionOverrides(tool.SourceURL, version)
	}

	return tool, nil
}

// versionOverride holds version override data from Aqua registry.
// Fields mirror Aqua's VersionOverride to handle all real-world registry YAML patterns.
type versionOverride struct {
	VersionConstraint   string                    `yaml:"version_constraint"`
	Type                string                    `yaml:"type"`
	RepoOwner           string                    `yaml:"repo_owner"`
	RepoName            string                    `yaml:"repo_name"`
	Asset               string                    `yaml:"asset"`
	URL                 string                    `yaml:"url"` // Alternative to Asset for http type tools.
	Format              string                    `yaml:"format"`
	FormatOverrides     []registry.FormatOverride `yaml:"format_overrides"`
	VersionPrefix       string                    `yaml:"version_prefix"`
	Replacements        map[string]string         `yaml:"replacements"`
	Overrides           []registry.AquaOverride   `yaml:"overrides"`
	Files               []registry.File           `yaml:"files"`
	SupportedEnvs       []string                  `yaml:"supported_envs"`
	Rosetta2            bool                      `yaml:"rosetta2"`
	WindowsArmEmulation bool                      `yaml:"windows_arm_emulation"`
	NoAsset             bool                      `yaml:"no_asset"`
	Checksum            registry.ChecksumConfig   `yaml:"checksum"`
	ErrorMessage        string                    `yaml:"error_message"`
}

// applyVersionOverride applies a version override to the tool.
// Only non-empty/non-zero override fields are applied; base fields serve as defaults.
// When the override changes the tool Type, resetByPkgType clears fields not applicable to the new type.
func applyVersionOverride(tool *registry.Tool, override *versionOverride, version string) {
	// If the override changes the type, reset type-specific fields first.
	if override.Type != "" && override.Type != tool.Type {
		resetByPkgType(tool, override.Type)
		tool.Type = override.Type
	}
	if override.RepoOwner != "" {
		tool.RepoOwner = override.RepoOwner
	}
	if override.RepoName != "" {
		tool.RepoName = override.RepoName
	}
	// Use Asset if specified, otherwise fall back to URL (used by http type tools).
	if override.Asset != "" {
		tool.Asset = override.Asset
	} else if override.URL != "" {
		tool.Asset = override.URL
	}
	if override.Format != "" {
		tool.Format = override.Format
	}
	if len(override.FormatOverrides) > 0 {
		tool.FormatOverrides = override.FormatOverrides
	}
	if override.VersionPrefix != "" {
		tool.VersionPrefix = override.VersionPrefix
	}
	if len(override.Files) > 0 {
		tool.Name = override.Files[0].Name
		tool.Files = override.Files
	}
	if len(override.SupportedEnvs) > 0 {
		tool.SupportedEnvs = override.SupportedEnvs
	}
	// Apply replacements from version override (replaces base replacements).
	if len(override.Replacements) > 0 {
		tool.Replacements = override.Replacements
	}
	// Apply overrides from version override (replaces base overrides).
	if len(override.Overrides) > 0 {
		tool.Overrides = convertAquaOverrides(override.Overrides)
	}
	// Apply rosetta2 and windows_arm_emulation flags (additive — once true, stays true).
	if override.Rosetta2 {
		tool.Rosetta2 = true
	}
	if override.WindowsArmEmulation {
		tool.WindowsArmEmulation = true
	}
	if override.ErrorMessage != "" {
		tool.ErrorMessage = override.ErrorMessage
	}
	log.Debug("Applied version override", versionLogKey, version, "constraint", override.VersionConstraint, "asset", tool.Asset, "format", tool.Format, "replacements", tool.Replacements)
}

// resetByPkgType clears fields not applicable when changing to a new package type.
// This matches upstream aquaproj/aqua behavior where switching types (e.g., github_release -> http)
// resets type-specific fields to avoid stale configuration.
func resetByPkgType(tool *registry.Tool, newType string) {
	switch newType {
	case "http":
		// HTTP type uses URL, not github_release-specific fields.
		tool.Asset = ""
	case "github_release":
		// GitHub release type uses Asset, not http-specific URL.
		tool.URL = ""
	}
}

// registryPackage holds package data from Aqua registry file.
// This struct must include all fields that need to be preserved when resolving version overrides.
type registryPackage struct {
	Name                string                    `yaml:"name"` // Package name (e.g., "kubernetes/kubernetes/kubectl").
	Type                string                    `yaml:"type"`
	RepoOwner           string                    `yaml:"repo_owner"`
	RepoName            string                    `yaml:"repo_name"`
	Asset               string                    `yaml:"asset"` // Used by github_release types.
	URL                 string                    `yaml:"url"`   // Used by http types.
	Format              string                    `yaml:"format"`
	FormatOverrides     []registry.FormatOverride `yaml:"format_overrides"`
	BinaryName          string                    `yaml:"binary_name"`
	Description         string                    `yaml:"description"`
	VersionPrefix       string                    `yaml:"version_prefix"`
	VersionConstraint   string                    `yaml:"version_constraint"` // Top-level version constraint.
	Rosetta2            bool                      `yaml:"rosetta2"`           // Allow arm64 to fall back to amd64 on macOS.
	WindowsArmEmulation bool                      `yaml:"windows_arm_emulation"`
	Replacements        map[string]string         `yaml:"replacements"`
	Overrides           []registry.AquaOverride   `yaml:"overrides"`
	Files               []registry.File           `yaml:"files"`
	VersionOverrides    []versionOverride         `yaml:"version_overrides"`
	SupportedEnvs       []string                  `yaml:"supported_envs"` // Supported platforms (e.g., "darwin", "linux").
	ErrorMessage        string                    `yaml:"error_message"`
	VersionSource       string                    `yaml:"version_source"` // Version source: "github_release" (default) or "github_tag".
	NoAsset             bool                      `yaml:"no_asset"`
}

// resolveVersionOverrides fetches the full registry file and resolves version-specific overrides.
// The sourceURL parameter should be the exact URL where the tool's registry.yaml was found,
// which handles nested paths like kubernetes/kubernetes/kubectl correctly.
//
// This follows upstream aquaproj/aqua's SetVersion() algorithm:
//  1. If NO top-level version_constraint -> return base package as-is (skip overrides).
//  2. If top-level version_constraint matches -> return base package.
//  3. Try each version_override (first match wins, apply override fields).
//  4. If nothing matched -> return base package (NOT an error).
func (ar *AquaRegistry) resolveVersionOverrides(sourceURL, version string) (*registry.Tool, error) {
	if sourceURL == "" {
		return nil, fmt.Errorf("%w: source URL is required for version override resolution", registry.ErrToolNotFound)
	}

	pkgDef, err := ar.fetchRegistryPackage(sourceURL)
	if err != nil {
		return nil, err
	}

	// Determine asset pattern: prefer Asset (github_release), fall back to URL (http type).
	asset := pkgDef.Asset
	if asset == "" {
		asset = pkgDef.URL
	}

	tool := &registry.Tool{
		Name:                resolveBinaryName(pkgDef.BinaryName, pkgDef.Name, pkgDef.RepoName),
		Type:                pkgDef.Type,
		RepoOwner:           pkgDef.RepoOwner,
		RepoName:            pkgDef.RepoName,
		Asset:               asset,
		Format:              pkgDef.Format,
		FormatOverrides:     pkgDef.FormatOverrides,
		BinaryName:          pkgDef.BinaryName,
		VersionPrefix:       pkgDef.VersionPrefix,
		Replacements:        pkgDef.Replacements,
		Overrides:           convertAquaOverrides(pkgDef.Overrides),
		Files:               pkgDef.Files,
		SourceURL:           sourceURL,
		SupportedEnvs:       pkgDef.SupportedEnvs,
		Rosetta2:            pkgDef.Rosetta2,
		WindowsArmEmulation: pkgDef.WindowsArmEmulation,
		ErrorMessage:        pkgDef.ErrorMessage,
		VersionSource:       pkgDef.VersionSource,
		NoAsset:             pkgDef.NoAsset,
	}

	// Phase 1: If no top-level constraint, return base (no overrides checked).
	// This matches upstream: when there's no version_constraint at all, overrides are skipped.
	if pkgDef.VersionConstraint == "" {
		log.Debug("No top-level version_constraint, returning base config", versionLogKey, version)
		return tool, nil
	}

	// Compute SemVer (prefix-stripped version) for constraint evaluation.
	sv := computeSemVer(version, pkgDef.VersionPrefix)

	// Phase 2: Evaluate top-level constraint. If it matches, return base config.
	matches, err := evaluateVersionConstraint(pkgDef.VersionConstraint, version, sv)
	if err == nil && matches {
		log.Debug("Version matches top-level constraint, returning base config",
			versionLogKey, version, "constraint", pkgDef.VersionConstraint)
		return tool, nil
	}

	// Phase 3: Try version overrides. First match wins.
	for i := range pkgDef.VersionOverrides {
		vo := &pkgDef.VersionOverrides[i]

		// Compute per-override SemVer (upstream supports version_prefix in overrides).
		vp := pkgDef.VersionPrefix
		if vo.VersionPrefix != "" {
			vp = vo.VersionPrefix
		}
		voSV := computeSemVer(version, vp)

		m, constraintErr := evaluateVersionConstraint(vo.VersionConstraint, version, voSV)
		if constraintErr != nil {
			log.Debug("Failed to evaluate version override constraint",
				"constraint", vo.VersionConstraint, versionLogKey, version, "error", constraintErr)
			continue
		}
		if m {
			applyVersionOverride(tool, vo, version)
			return tool, nil
		}
	}

	// Phase 4: No match -> return base config (NOT an error).
	// This matches upstream: unmatched versions get the base package configuration.
	log.Debug("No matching version override, returning base config",
		versionLogKey, version, "overrides_count", len(pkgDef.VersionOverrides))
	return tool, nil
}

// computeSemVer strips the version prefix to produce a SemVer-compatible string.
// If the prefix is empty or the version doesn't start with it, returns the version as-is.
func computeSemVer(version, prefix string) string {
	if prefix != "" && strings.HasPrefix(version, prefix) {
		return strings.TrimPrefix(version, prefix)
	}
	return version
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

// defaultMkdirPermissions moved to constants block.

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
		tool, err := ar.parseRegistryFile(data)
		if err != nil {
			return nil, err
		}
		tool.SourceURL = url
		return tool, nil
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

	tool, err := ar.parseRegistryFile(data)
	if err != nil {
		return nil, err
	}
	tool.SourceURL = url
	return tool, nil
}

// parseRegistryFile parses Aqua registry YAML data.
func (ar *AquaRegistry) parseRegistryFile(data []byte) (*registry.Tool, error) {
	// Try AquaRegistryFile (packages)
	var aquaRegistry registry.AquaRegistryFile
	if err := yaml.Unmarshal(data, &aquaRegistry); err == nil && len(aquaRegistry.Packages) > 0 {
		pkg := aquaRegistry.Packages[0]
		// Determine asset pattern: prefer Asset (github_release), fall back to URL (http type).
		asset := pkg.Asset
		if asset == "" {
			asset = pkg.URL
		}

		// Convert AquaPackage to Tool.
		tool := &registry.Tool{
			Name:                resolveBinaryName(pkg.BinaryName, pkg.Name, pkg.RepoName),
			RepoOwner:           pkg.RepoOwner,
			RepoName:            pkg.RepoName,
			Asset:               asset,
			Format:              pkg.Format,
			FormatOverrides:     pkg.FormatOverrides,
			Type:                pkg.Type,
			BinaryName:          pkg.BinaryName,
			VersionPrefix:       pkg.VersionPrefix,
			Rosetta2:            pkg.Rosetta2,
			WindowsArmEmulation: pkg.WindowsArmEmulation,
			ErrorMessage:        pkg.ErrorMessage,
			VersionSource:       pkg.VersionSource,
			NoAsset:             pkg.NoAsset,
			// Copy Aqua-specific fields for nested file extraction and platform overrides.
			Files:         pkg.Files,
			Replacements:  pkg.Replacements,
			Overrides:     convertAquaOverrides(pkg.Overrides),
			SupportedEnvs: pkg.SupportedEnvs,
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

// convertAquaOverrides converts Aqua overrides to the internal Override format.
func convertAquaOverrides(aquaOverrides []registry.AquaOverride) []registry.Override {
	if len(aquaOverrides) == 0 {
		return nil
	}
	overrides := make([]registry.Override, len(aquaOverrides))
	for i, ao := range aquaOverrides {
		overrides[i] = registry.Override{
			GOOS:         ao.GOOS,
			GOARCH:       ao.GOARCH,
			Envs:         ao.Envs,
			Type:         ao.Type,
			Asset:        ao.Asset,
			URL:          ao.URL,
			Format:       ao.Format,
			Files:        ao.Files,
			Replacements: ao.Replacements,
		}
		// If URL is set in Aqua override and Asset is not, use URL as Asset.
		if ao.URL != "" && ao.Asset == "" {
			overrides[i].Asset = ao.URL
		}
	}
	return overrides
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
// Following Aqua behavior: version_prefix defaults to empty, not "v".
// Templates use {{trimV .Version}} or {{.SemVer}} when they need the version without prefix.
func resolveVersionStrings(tool *registry.Tool, version string) (releaseVersion, semVer string) {
	// Use version_prefix only if explicitly set in registry definition.
	// This matches Aqua's behavior where version_prefix defaults to empty.
	prefix := tool.VersionPrefix

	releaseVersion = version
	if prefix != "" && !strings.HasPrefix(releaseVersion, prefix) {
		releaseVersion = prefix + releaseVersion
	}

	// SemVer is the version without any prefix.
	semVer = version
	if prefix != "" {
		semVer = strings.TrimPrefix(releaseVersion, prefix)
	}
	return releaseVersion, semVer
}

// buildAssetTemplateData creates the template data map for asset URL rendering.
// Applies tool.Replacements to OS/Arch values, matching the installer's buildTemplateData behavior.
func buildAssetTemplateData(tool *registry.Tool, releaseVersion, semVer string) map[string]string {
	// Use the tool's explicit format. Aqua defaults to empty for github_release/http types,
	// meaning raw binary (no archive extraction needed).
	format := tool.Format

	// Apply per-OS format overrides (e.g., zip on Windows, tar.gz on Linux).
	for _, fo := range tool.FormatOverrides {
		if fo.GOOS == getOS() {
			format = fo.Format
			break
		}
	}

	// Get OS and Arch, applying emulation fallbacks and replacements.
	osVal := getOS()
	archVal := getArch()

	// Rosetta 2 fallback: on darwin/arm64, always use amd64 when rosetta2 is enabled.
	// This matches upstream aquaproj/aqua behavior (no arm64 replacement check).
	if tool.Rosetta2 && osVal == "darwin" && archVal == "arm64" {
		archVal = "amd64"
	}

	// Windows ARM emulation fallback: always use amd64 when enabled on windows/arm64.
	if tool.WindowsArmEmulation && osVal == "windows" && archVal == "arm64" {
		archVal = "amd64"
	}

	// Apply replacements from the tool config.
	if tool.Replacements != nil {
		if replacement, ok := tool.Replacements[osVal]; ok {
			osVal = replacement
		}
		if replacement, ok := tool.Replacements[archVal]; ok {
			archVal = replacement
		}
	}

	return map[string]string{
		"Version":   releaseVersion,
		"SemVer":    semVer,
		"OS":        osVal,
		"Arch":      archVal,
		"GOOS":      getOS(),
		"GOARCH":    getArch(),
		"RepoOwner": tool.RepoOwner,
		"RepoName":  tool.RepoName,
		"Format":    format,
	}
}

// assetTemplateFuncs returns the template function map for asset URL templates.
// Uses Sprig v3 text functions as the base (matching Aqua upstream), with Aqua-specific overrides.
func assetTemplateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()

	// Override with Aqua-specific functions that have different argument order
	// or behavior than Sprig equivalents.
	funcs["trimV"] = func(s string) string {
		return strings.TrimPrefix(s, versionPrefix)
	}
	funcs["trimPrefix"] = func(prefix, s string) string {
		return strings.TrimPrefix(s, prefix)
	}
	funcs["trimSuffix"] = func(suffix, s string) string {
		return strings.TrimSuffix(s, suffix)
	}
	funcs["replace"] = func(old, new, s string) string {
		return strings.ReplaceAll(s, old, new)
	}

	return funcs
}

// executeAssetTemplate parses and executes the asset template with two-pass rendering.
// Pass 1: Render with base variables (Version, SemVer, OS, Arch, etc.).
// Pass 2: If the template references {{.Asset}} or {{.AssetWithoutExt}},
// inject the rendered asset name and re-render.
func executeAssetTemplate(assetTemplate string, data map[string]string) (string, error) {
	// Pass 1: Render with base data.
	result, err := renderTemplate(assetTemplate, data)
	if err != nil {
		return "", err
	}

	// Pass 2: If template references Asset or AssetWithoutExt, re-render with those values.
	if strings.Contains(assetTemplate, ".Asset") {
		data["Asset"] = result
		data["AssetWithoutExt"] = stripFileExtension(result)
		result, err = renderTemplate(assetTemplate, data)
		if err != nil {
			return "", err
		}
	}

	return result, nil
}

// renderTemplate parses and executes a Go template string.
func renderTemplate(templateStr string, data map[string]string) (string, error) {
	tmpl, err := template.New("asset").Funcs(assetTemplateFuncs()).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to parse asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: failed to execute asset template: %w", registry.ErrNoAssetTemplate, err)
	}

	return buf.String(), nil
}

// stripFileExtension removes the file extension from an asset name.
// Handles compound extensions like .tar.gz, .tar.xz, etc.
func stripFileExtension(name string) string {
	// Check compound extensions first.
	compoundExts := []string{".tar.gz", ".tar.xz", ".tar.bz2"}
	lower := strings.ToLower(name)
	for _, ext := range compoundExts {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	// Fall back to single extension.
	ext := filepath.Ext(name)
	if ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// extractBinaryNameFromPackageName extracts the binary name from an Aqua package name.
// Aqua package names follow the pattern "owner/repo" or "owner/repo/binary".
// For example: "kubernetes/kubernetes/kubectl" -> "kubectl"
// For example: "hashicorp/terraform" -> "" (caller should fall back to repo_name).
func extractBinaryNameFromPackageName(packageName string) string {
	if packageName == "" {
		return ""
	}
	parts := strings.Split(packageName, "/")
	// If there are more than 2 segments, the last one is the binary name.
	// E.g., "kubernetes/kubernetes/kubectl" has 3 parts, so "kubectl" is the binary.
	if len(parts) > 2 {
		return parts[len(parts)-1]
	}
	// For 2-segment names like "hashicorp/terraform", return empty
	// so the caller falls back to repo_name.
	return ""
}

// resolveBinaryName determines the binary name using Aqua's resolution order:
// 1. Use explicit binary_name if set
// 2. Extract last segment from package name (e.g., "kubectl" from "kubernetes/kubernetes/kubectl")
// 3. Fall back to repo_name.
func resolveBinaryName(binaryName, packageName, repoName string) string {
	if binaryName != "" {
		return binaryName
	}
	if name := extractBinaryNameFromPackageName(packageName); name != "" {
		return name
	}
	return repoName
}
