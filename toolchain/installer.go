package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/gabriel-vasile/mimetype"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/xdg"
	"github.com/cloudposse/atmos/toolchain/registry"
)

const (
	versionPrefix               = "v"
	defaultFileWritePermissions = 0o644
	defaultMkdirPermissions     = 0o755
	maxUnixPermissions          = 0o7777
	maxDecompressedSizeMB       = 3000
	bufferSizeBytes             = 32 * 1024
)

// ToolResolver defines an interface for resolving tool names to owner/repo pairs
// This allows for mocking in tests and flexible resolution in production.
type ToolResolver interface {
	Resolve(toolName string) (owner, repo string, err error)
}

// DefaultToolResolver implements ToolResolver using configured aliases and registry search.
type DefaultToolResolver struct {
	atmosConfig *schema.AtmosConfiguration
}

func (d *DefaultToolResolver) Resolve(toolName string) (string, string, error) {
	defer perf.Track(nil, "toolchain.DefaultToolResolver.Resolve")()

	// Step 1: Check if this is an alias in atmos.yaml.
	// If atmosConfig is available and has aliases configured, resolve the alias first.
	if d.atmosConfig != nil && len(d.atmosConfig.Toolchain.Aliases) > 0 {
		if aliasedName, found := d.atmosConfig.Toolchain.Aliases[toolName]; found {
			toolName = aliasedName
		}
	}

	// Step 2: If already in owner/repo format, parse and return.
	if strings.Contains(toolName, "/") {
		parts := strings.Split(toolName, "/")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	// Step 3: Try to find the tool in the Aqua registry.
	owner, repo, err := searchRegistryForTool(toolName)
	if err == nil {
		return owner, repo, nil
	}
	return "", "", errUtils.Build(errUtils.ErrToolNotInRegistry).
		WithExplanationf("Tool `%s` not found in Aqua registry", toolName).
		WithHint("Use full format: `owner/repo` (e.g., `hashicorp/terraform`)").
		WithHint("Run `atmos toolchain registry search` to browse available tools").
		WithContext("tool", toolName).
		WithExitCode(2).
		Err()
}

// Installer handles the installation of CLI binaries.
type Installer struct {
	registryPath string
	cacheDir     string
	binDir       string
	registries   []string
	resolver     ToolResolver
}

// NewInstallerWithResolver allows injecting a custom ToolResolver (for tests).
func NewInstallerWithResolver(resolver ToolResolver) *Installer {
	defer perf.Track(nil, "toolchain.NewInstaller")()

	// Use XDG-compliant cache directory.
	cacheDir, err := xdg.GetXDGCacheDir("toolchain", 0o755)
	if err != nil || cacheDir == "" {
		// Fallback to manual construction if XDG fails.
		log.Warn("XDG cache dir unavailable, falling back to manual construction", "error", err)
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			log.Warn("Falling back to temp dir for cache", "error", homeErr)
			cacheDir = filepath.Join(os.TempDir(), ".cache", "tools-cache")
		} else {
			cacheDir = filepath.Join(homeDir, ".cache", "tools-cache")
		}
	}
	binDir := filepath.Join(GetInstallPath(), "bin")
	registries := []string{
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
		"./tool-registry",
	}
	return &Installer{
		registryPath: "./tool-registry",
		cacheDir:     cacheDir,
		binDir:       binDir,
		registries:   registries,
		resolver:     resolver,
	}
}

// NewInstaller uses the default resolver with alias support from atmosConfig.
func NewInstaller() *Installer {
	defer perf.Track(nil, "toolchain.NewInstaller")()

	// Get the current atmos config to support alias resolution.
	return NewInstallerWithResolver(&DefaultToolResolver{
		atmosConfig: GetAtmosConfig(),
	})
}

// Install installs a tool from the registry.
func (i *Installer) Install(owner, repo, version string) (string, error) {
	defer perf.Track(nil, "toolchain.Install")()

	// Get tool from registry
	tool, err := i.findTool(owner, repo, version)
	if err != nil {
		return "", err // Error already enriched in findTool
	}
	return i.installFromTool(tool, version)
}

// Helper to handle the rest of the install logic.
func (i *Installer) installFromTool(tool *registry.Tool, version string) (string, error) {
	assetURL, err := i.buildAssetURL(tool, version)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToolSpec, err)
	}
	log.Debug("Downloading tool", "owner", tool.RepoOwner, "repo", tool.RepoName, "version", version, "url", assetURL)

	assetPath, err := i.downloadAssetWithVersionFallback(tool, version, assetURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrHTTPRequest, err)
	}
	binaryPath, err := i.extractAndInstall(tool, assetPath, version)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrFileOperation, err)
	}
	if err := os.Chmod(binaryPath, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to make binary executable: %w", ErrFileOperation, err)
	}
	// Set mod time to now so install date reflects installation, not archive timestamp
	now := time.Now()
	_ = os.Chtimes(binaryPath, now, now)
	return binaryPath, nil
}

// findTool searches for a tool in the registry.
func (i *Installer) findTool(owner, repo, version string) (*registry.Tool, error) {
	defer perf.Track(nil, "toolchain.findTool")()

	// Search through all registries
	for _, registry := range i.registries {
		tool, err := i.searchRegistry(registry, owner, repo, version)
		if err == nil {
			return tool, nil
		}
	}

	// Build list of registry names for context
	registryNames := make([]string, len(i.registries))
	copy(registryNames, i.registries)

	return nil, errUtils.Build(errUtils.ErrToolNotInRegistry).
		WithExplanationf("Tool `%s/%s@%s` was not found in any configured registry. "+
			"Atmos searches registries in priority order: Atmos registry → Aqua registry → custom registries.", owner, repo, version).
		WithHint("Run `atmos toolchain registry search` to browse available tools").
		WithHint("Verify network connectivity to registries").
		WithHint("Check registry configuration in `atmos.yaml`").
		WithContext("tool", fmt.Sprintf("%s/%s@%s", owner, repo, version)).
		WithContext("registries_searched", strings.Join(registryNames, ", ")).
		WithExitCode(2).
		Err()
}

// searchRegistry searches a specific registry for a tool.
// Version is required to apply version-specific overrides from the registry.
func (i *Installer) searchRegistry(registry, owner, repo, version string) (*registry.Tool, error) {
	// Try to fetch from Aqua registry for remote registries
	if strings.HasPrefix(registry, "http") {
		// Use the Aqua registry implementation
		ar := NewAquaRegistry()
		tool, err := ar.GetToolWithVersion(owner, repo, version)
		if err != nil {
			return nil, err
		}
		// Ensure RepoOwner and RepoName are set correctly
		tool.RepoOwner = owner
		tool.RepoName = repo
		return tool, nil
	}

	// Try local registry
	return i.searchLocalRegistry(registry, owner, repo)
}

// searchLocalRegistry searches a local registry for a tool.
func (i *Installer) searchLocalRegistry(registryPath, owner, repo string) (*registry.Tool, error) {
	toolFile := filepath.Join(registryPath, owner, repo+".yaml")
	if _, err := os.Stat(toolFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: tool file not found: %s", ErrToolNotFound, toolFile)
	}

	return i.loadToolFile(toolFile)
}

// loadToolFile loads a tool YAML file.
func (i *Installer) loadToolFile(filePath string) (*registry.Tool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var registryFile struct {
		Packages []registry.Tool `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &registryFile); err != nil {
		return nil, err
	}

	// Return the first tool (assuming single tool per file)
	if len(registryFile.Packages) > 0 {
		return &registryFile.Packages[0], nil
	}

	return nil, fmt.Errorf("%w: no tools found in %s", ErrToolNotFound, filePath)
}

// parseToolSpec parses a tool specification (owner/repo or just repo).
func (i *Installer) parseToolSpec(tool string) (string, string, error) {
	defer perf.Track(nil, "toolchain.parseToolSpec")()

	parts := strings.Split(tool, "/")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	} else if len(parts) == 1 {
		return i.resolver.Resolve(parts[0])
	}
	return "", "", fmt.Errorf("%w: invalid tool specification: %s", ErrInvalidToolSpec, tool)
}

// buildAssetURL constructs the download URL for the asset.
func (i *Installer) buildAssetURL(tool *registry.Tool, version string) (string, error) {
	// Handle different tool types
	switch tool.Type {
	case "http":
		// For HTTP type, the Asset field contains the full URL template
		if tool.Asset == "" {
			return "", fmt.Errorf("%w: Asset URL template is required for HTTP type tools", ErrInvalidToolSpec)
		}

		// Remove 'v' prefix from version for asset naming
		versionForAsset := version
		if strings.HasPrefix(versionForAsset, versionPrefix) {
			versionForAsset = versionForAsset[1:]
		}

		// Create template data
		data := struct {
			Version   string
			OS        string
			Arch      string
			RepoOwner string
			RepoName  string
			Format    string
		}{
			Version:   versionForAsset,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			RepoOwner: tool.RepoOwner,
			RepoName:  tool.RepoName,
			Format:    tool.Format,
		}

		// Register custom template functions
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

		tmpl, err := template.New("asset").Funcs(funcMap).Parse(tool.Asset)
		if err != nil {
			return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
				WithExplanationf("Asset template for `%s/%s` contains invalid Go template syntax", tool.RepoOwner, tool.RepoName).
				WithExample("# Valid asset template:\nasset: \"https://releases.example.com/{{.RepoName}}/v{{.Version}}/{{.RepoName}}_{{.OS}}_{{.Arch}}.tar.gz\"").
				WithHint("Check the tool definition in the registry for syntax errors").
				WithHint("Verify template variables: {{.Version}}, {{.OS}}, {{.Arch}}, {{.RepoOwner}}, {{.RepoName}}").
				WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
				WithContext("template", tool.Asset).
				WithContext("parse_error", err.Error()).
				WithExitCode(2).
				Err()
		}

		var url strings.Builder
		if err := tmpl.Execute(&url, data); err != nil {
			return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
				WithExplanationf("Asset template for `%s/%s` failed to execute", tool.RepoOwner, tool.RepoName).
				WithHint("Template executed but produced invalid output").
				WithHint("Check template logic and variable availability").
				WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
				WithContext("template", tool.Asset).
				WithContext("execution_error", err.Error()).
				WithExitCode(2).
				Err()
		}

		return url.String(), nil

	case "github_release":
		// For GitHub releases, validate that RepoOwner and RepoName are set
		if tool.RepoOwner == "" || tool.RepoName == "" {
			return "", fmt.Errorf("%w: RepoOwner and RepoName must be set for github_release type (got RepoOwner=%q, RepoName=%q)", ErrInvalidToolSpec, tool.RepoOwner, tool.RepoName)
		}

		// Use the asset template from the tool
		assetTemplate := tool.Asset
		if assetTemplate == "" {
			// Fallback to a common pattern
			assetTemplate = "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
		}

		// Remove 'v' prefix from version for asset naming
		versionForAsset := version
		versionForAsset = strings.TrimPrefix(versionForAsset, versionPrefix)

		// Create template data
		data := struct {
			Version   string
			OS        string
			Arch      string
			RepoOwner string
			RepoName  string
			Format    string
		}{
			Version:   versionForAsset,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			RepoOwner: tool.RepoOwner,
			RepoName:  tool.RepoName,
			Format:    tool.Format,
		}

		// Register custom template functions
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

		tmpl, err := template.New("asset").Funcs(funcMap).Parse(assetTemplate)
		if err != nil {
			return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
				WithExplanationf("Asset template for `%s/%s` contains invalid Go template syntax", tool.RepoOwner, tool.RepoName).
				WithExample("# Valid asset template:\nasset: \"{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz\"").
				WithHint("Check the tool definition in the registry for syntax errors").
				WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
				WithContext("template", assetTemplate).
				WithContext("parse_error", err.Error()).
				WithExitCode(2).
				Err()
		}

		var assetName strings.Builder
		if err := tmpl.Execute(&assetName, data); err != nil {
			return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
				WithExplanationf("Asset template for `%s/%s` failed to execute", tool.RepoOwner, tool.RepoName).
				WithHint("Template executed but produced invalid output").
				WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
				WithContext("template", assetTemplate).
				WithContext("execution_error", err.Error()).
				WithExitCode(2).
				Err()
		}

		// Construct the full GitHub release URL
		url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
			tool.RepoOwner, tool.RepoName, version, assetName.String())

		return url, nil

	default:
		return "", fmt.Errorf("%w: unsupported tool type: %s", ErrInvalidToolSpec, tool.Type)
	}
}

// downloadAsset downloads an asset to the cache directory.
func (i *Installer) downloadAsset(url string) (string, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(i.cacheDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create cache directory: %w", ErrFileOperation, err)
	}

	// Extract filename from URL
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]
	cachePath := filepath.Join(i.cacheDir, filename)

	// Check if already cached
	if _, err := os.Stat(cachePath); err == nil {
		log.Debug("Using cached asset", "filename", filename)
		return cachePath, nil
	}

	// Download the file using authenticated HTTP client
	log.Debug("Downloading asset", "filename", filename)
	client := httpClient.NewDefaultClient(
		httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %w", ErrHTTPRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDownloadFailed).
			WithExplanationf("Failed to download asset from `%s`", url).
			WithHint("Check your internet connection").
			WithHint("Verify GitHub access: `curl -I https://api.github.com`").
			WithHint("If behind proxy, ensure `HTTPS_PROXY` environment variable is set").
			WithContext("url", url).
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		builder := errUtils.Build(errUtils.ErrDownloadFailed).
			WithExplanationf("Download failed with HTTP %d", resp.StatusCode).
			WithContext("url", url).
			WithContext("status_code", resp.StatusCode).
			WithExitCode(1)

		switch resp.StatusCode {
		case http.StatusNotFound:
			builder.
				WithHint("Asset not found - check tool name and version are correct").
				WithHint("Try without `v` prefix: `@1.5.0` instead of `@v1.5.0`")
		case http.StatusForbidden, http.StatusUnauthorized:
			builder.
				WithHint("GitHub API rate limit exceeded or authentication required").
				WithHint("Set `GITHUB_TOKEN` environment variable to increase rate limits").
				WithHint("Get token at: https://github.com/settings/tokens")
		default:
			builder.WithHint("Check GitHub status: https://www.githubstatus.com")
		}

		return "", builder.Err()
	}

	// Read response body into memory.
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read response body: %w", ErrHTTPRequest, err)
	}

	// Write atomically to cache using filesystem package.
	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(cachePath, buf.Bytes(), defaultFileWritePermissions); err != nil {
		return "", fmt.Errorf("%w: failed to write cache file atomically: %w", ErrFileOperation, err)
	}

	return cachePath, nil
}

// downloadAssetWithVersionFallback tries the asset URL as-is, then with 'v' prefix or without, if 404.
func (i *Installer) downloadAssetWithVersionFallback(tool *registry.Tool, version, assetURL string) (string, error) {
	assetPath, err := i.downloadAsset(assetURL)
	if err == nil {
		return assetPath, nil
	}
	if !isHTTP404(err) {
		return "", err
	}
	// Try fallback with or without 'v'
	var fallbackVersion string
	if strings.HasPrefix(version, versionPrefix) {
		fallbackVersion = strings.TrimPrefix(version, versionPrefix)
	} else {
		fallbackVersion = versionPrefix + version
	}
	if fallbackVersion == version {
		return "", err // nothing to try
	}
	fallbackURL, buildErr := i.buildAssetURL(tool, fallbackVersion)
	if buildErr != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToolSpec, buildErr)
	}
	log.Debug("Asset 404, trying fallback version", "original", assetURL, "fallback", fallbackURL)
	assetPath, err = i.downloadAsset(fallbackURL)
	if err == nil {
		return assetPath, nil
	}
	return "", fmt.Errorf("%w: tried %s and %s: %w", ErrHTTPRequest, assetURL, fallbackURL, err)
}

// isHTTP404 returns true if the error is a 404 from downloadAsset.
func isHTTP404(err error) bool {
	return errors.Is(err, ErrHTTP404)
}

// extractAndInstall extracts the binary from the asset and installs it.
func (i *Installer) extractAndInstall(tool *registry.Tool, assetPath, version string) (string, error) {
	// Create version-specific directory
	versionDir := filepath.Join(i.binDir, tool.RepoOwner, tool.RepoName, version)
	if err := os.MkdirAll(versionDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create version directory: %w", ErrFileOperation, err)
	}

	// Determine the binary name
	binaryName := tool.BinaryName
	if binaryName == "" {
		binaryName = tool.Name
	}
	if binaryName == "" {
		binaryName = tool.RepoName
	}

	binaryPath := filepath.Join(versionDir, binaryName)

	// For now, just copy the file (simplified extraction)
	if err := i.simpleExtract(assetPath, binaryPath, tool); err != nil {
		return "", fmt.Errorf("%w: %w", ErrFileOperation, err)
	}

	return binaryPath, nil
}

// simpleExtract is a robust extraction method using magic file type detection.
func (i *Installer) simpleExtract(assetPath, binaryPath string, tool *registry.Tool) error {
	// Detect file type using magic bytes
	mime, err := mimetype.DetectFile(assetPath)
	if err != nil {
		return fmt.Errorf("%w: failed to detect file type: %w", ErrFileOperation, err)
	}

	log.Debug("Detected file type", "mime", mime.String(), "extension", mime.Extension())

	switch {
	case mime.Is("application/zip"):
		return i.extractZip(assetPath, binaryPath, tool)
	case mime.Is("application/x-gzip") || mime.Is("application/gzip"):
		// Check if it's a tar.gz (by extension or by magic)
		if strings.HasSuffix(assetPath, ".tar.gz") || strings.HasSuffix(assetPath, ".tgz") || mime.Is("application/x-tar") {
			return i.extractTarGz(assetPath, binaryPath, tool)
		}
		// Otherwise, treat as a single gzip-compressed binary
		return i.extractGzip(assetPath, binaryPath)
	case mime.Is("application/x-tar"):
		return i.extractTarGz(assetPath, binaryPath, tool)
	case mime.Is("application/octet-stream") || mime.Is("application/x-executable"):
		return i.copyFile(assetPath, binaryPath)
	default:
		// Fallback to extension-based detection
		if strings.HasSuffix(assetPath, ".zip") {
			return i.extractZip(assetPath, binaryPath, tool)
		}
		if strings.HasSuffix(assetPath, ".tar.gz") || strings.HasSuffix(assetPath, ".tgz") {
			return i.extractTarGz(assetPath, binaryPath, tool)
		}
		if strings.HasSuffix(assetPath, ".gz") {
			return i.extractGzip(assetPath, binaryPath)
		}
		log.Debug("Unknown file type, copying as binary", "filename", filepath.Base(assetPath))
		return i.copyFile(assetPath, binaryPath)
	}
}

// extractZip extracts a ZIP file.
func (i *Installer) extractZip(zipPath, binaryPath string, tool *registry.Tool) error {
	log.Debug("Extracting ZIP archive", "filename", filepath.Base(zipPath))

	tempDir, err := os.MkdirTemp("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp dir: %w", ErrFileOperation, err)
	}
	defer os.RemoveAll(tempDir)

	err = Unzip(zipPath, tempDir)
	if err != nil {
		return fmt.Errorf("%w: failed to extract ZIP: %w", ErrFileOperation, err)
	}

	binaryName := tool.BinaryName
	if binaryName == "" {
		binaryName = tool.Name
	}
	if binaryName == "" {
		binaryName = tool.RepoName
	}

	// Find the binary in the temp dir (recursively)
	var found string
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && (info.Name() == binaryName || info.Name() == binaryName+".exe") {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: failed to search extracted files: %w", ErrFileOperation, err)
	}
	if found == "" {
		return fmt.Errorf("%w: binary %s not found in extracted archive", ErrToolNotFound, binaryName)
	}

	// Ensure the destination directory exists
	dir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(dir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create destination directory: %w", ErrFileOperation, err)
	}

	// Move the binary into place
	if err := MoveFile(found, binaryPath); err != nil {
		return fmt.Errorf("%w: failed to move extracted binary: %w", ErrFileOperation, err)
	}

	return nil
}

// Unzip extracts a zip archive to a destination directory.
// Works on Windows, macOS, and Linux.
func Unzip(src, dest string) error {
	defer perf.Track(nil, "toolchain.Unzip")()

	const maxDecompressedSize = maxDecompressedSizeMB * 1024 * 1024 // 3000MB limit per file

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipFile(f, dest, maxDecompressedSize); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dest string, maxSize int64) error {
	fpath, err := validatePath(f.Name, dest)
	if err != nil {
		return err
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(fpath, os.ModePerm)
	}

	if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
		return err
	}

	return copyFileContents(f, fpath, maxSize)
}

func validatePath(name, dest string) (string, error) {
	fpath := filepath.Join(dest, name)
	if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: illegal file path: %s", ErrFileOperation, name)
	}
	return fpath, nil
}

func copyFileContents(f *zip.File, fpath string, maxSize int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	return copyWithLimit(rc, outFile, f.Name, maxSize)
}

func copyWithLimit(src io.Reader, dst io.Writer, name string, maxSize int64) error {
	var totalBytes int64
	buf := make([]byte, bufferSizeBytes)

	for {
		n, err := src.Read(buf)
		totalBytes += int64(n)

		if totalBytes > maxSize {
			return fmt.Errorf("%w: decompressed size of %s exceeds limit: %d > %d", ErrFileOperation, name, totalBytes, maxSize)
		}

		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ExtractTarGz extracts a .tar.gz file to the given destination directory.
func ExtractTarGz(src, dest string) error {
	defer perf.Track(nil, "toolchain.ExtractTarGz")()

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%w: failed to open source file: %w", ErrFileOperation, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%w: failed to create gzip reader: %w", ErrFileOperation, err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: error reading tar: %w", ErrFileOperation, err)
		}

		if err := extractEntry(tr, header, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(tr *tar.Reader, header *tar.Header, dest string) error {
	//nolint:gosec // G305: Path is validated by isSafePath check on next line
	targetPath := filepath.Join(dest, header.Name)
	if !isSafePath(targetPath, dest) {
		return fmt.Errorf("%w: illegal file path: %s", ErrFileOperation, header.Name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return extractDir(targetPath, header)
	case tar.TypeReg:
		return extractFile(tr, targetPath, header)
	default:
		_ = ui.Warningf("Skipping unknown type: %s", header.Name)
		return nil
	}
}

func isSafePath(path, dest string) bool {
	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	return strings.HasPrefix(filepath.Clean(path), cleanDest)
}

func extractDir(path string, header *tar.Header) error {
	// Validate header.Mode
	if header.Mode < 0 || header.Mode > maxUnixPermissions { // Restrict to typical Unix permissions
		return fmt.Errorf("%w: invalid mode %d for %s: must be between 0 and %o", ErrFileOperation, header.Mode, path, maxUnixPermissions)
	}

	// Safe conversion to os.FileMode
	return os.MkdirAll(path, os.FileMode(header.Mode))
}

func extractFile(tr *tar.Reader, path string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(path), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	// Validate header.Mode is within uint32 range
	if header.Mode < 0 || header.Mode > math.MaxUint32 {
		return fmt.Errorf("%w: header.Mode out of uint32 range: %d", ErrFileOperation, header.Mode)
	}

	outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
	if err != nil {
		return fmt.Errorf("%w: failed to create file: %w", ErrFileOperation, err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, tr); err != nil {
		return fmt.Errorf("%w: failed to write file: %w", ErrFileOperation, err)
	}
	return nil
}

// extractTarGz extracts a tar.gz file.
func (i *Installer) extractTarGz(tarPath, binaryPath string, tool *registry.Tool) error {
	log.Debug("Extracting tar.gz archive", "filename", filepath.Base(tarPath))

	tempDir, err := os.MkdirTemp("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp dir: %w", ErrFileOperation, err)
	}
	defer os.RemoveAll(tempDir)

	if err = ExtractTarGz(tarPath, tempDir); err != nil {
		return fmt.Errorf("%w: failed to extract tar.gz: %w", ErrFileOperation, err)
	}

	binaryName := tool.BinaryName
	if binaryName == "" {
		binaryName = tool.Name
	}
	if binaryName == "" {
		binaryName = tool.RepoName
	}

	// Find the binary in the temp dir (recursively)
	var found string
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && (info.Name() == binaryName || info.Name() == binaryName+".exe") {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: failed to search extracted files: %w", ErrFileOperation, err)
	}
	if found == "" {
		return fmt.Errorf("%w: binary %s not found in extracted archive", ErrToolNotFound, binaryName)
	}

	// Ensure the destination directory exists
	dir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(dir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create destination directory: %w", ErrFileOperation, err)
	}

	// Move the binary into place
	if err := MoveFile(found, binaryPath); err != nil {
		return fmt.Errorf("%w: failed to move extracted binary: %w", ErrFileOperation, err)
	}

	return nil
}

// MoveFile tries os.Rename, but if that fails due to cross-device link,
// it falls back to a copy+remove.
func MoveFile(src, dst string) error {
	defer perf.Track(nil, "toolchain.MoveFile")()

	// Ensure target dir exists
	if err := os.MkdirAll(filepath.Dir(dst), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create target dir: %w", ErrFileOperation, err)
	}

	if err := os.Rename(src, dst); err != nil {
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("%w: failed to copy during move fallback: %w", ErrFileOperation, err)
		}
		if err := os.Remove(src); err != nil {
			return fmt.Errorf("%w: failed to remove source after copy: %w", ErrFileOperation, err)
		}
		return nil
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// extractGzip decompresses a single gzip-compressed binary.
func (i *Installer) extractGzip(gzPath, binaryPath string) error {
	log.Debug("Decompressing gzip binary", "filename", filepath.Base(gzPath))

	in, err := os.Open(gzPath)
	if err != nil {
		return fmt.Errorf("%w: failed to open gzip file: %w", ErrFileOperation, err)
	}
	defer in.Close()

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("%w: failed to create gzip reader: %w", ErrFileOperation, err)
	}
	defer gzr.Close()

	out, err := os.Create(binaryPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create output file: %w", ErrFileOperation, err)
	}
	defer out.Close()

	//nolint:gosec // G110: Single binary extraction from trusted GitHub releases, size limited by GitHub's release size limits
	if _, err := io.Copy(out, gzr); err != nil {
		return fmt.Errorf("%w: failed to decompress gzip: %w", ErrFileOperation, err)
	}

	return nil
}

// copyFile copies a file.
func (i *Installer) copyFile(src, dst string) error {
	log.Debug("Copying binary", "src", filepath.Base(src), "dst", filepath.Base(dst))

	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%w: failed to open source file: %w", ErrFileOperation, err)
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("%w: failed to create destination file: %w", ErrFileOperation, err)
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return fmt.Errorf("%w: failed to copy file: %w", ErrFileOperation, err)
	}

	return nil
}

// getBinaryPath returns the path to a specific version of a binary.
// If binaryName is provided and non-empty, it will be used directly.
// Otherwise, it will search the version directory for an executable file,
// falling back to using the repo name as the binary name.
func (i *Installer) getBinaryPath(owner, repo, version, binaryName string) string {
	versionDir := filepath.Join(i.binDir, owner, repo, version)

	// If binary name is explicitly provided, use it directly.
	if binaryName != "" {
		return filepath.Join(versionDir, binaryName)
	}

	// Try to find the actual binary in the version directory.
	// Some tools have different binary names than the repo name (e.g., opentofu -> tofu).
	entries, err := os.ReadDir(versionDir)
	if err == nil && len(entries) > 0 {
		// Look for an executable file.
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			entryPath := filepath.Join(versionDir, entry.Name())
			info, err := os.Stat(entryPath)
			if err == nil && info.Mode()&0o111 != 0 {
				// Found an executable.
				return entryPath
			}
		}
	}

	// Fallback: use repo name as binary name.
	return filepath.Join(versionDir, repo)
}

// Uninstall removes a previously installed tool.
func (i *Installer) Uninstall(owner, repo, version string) error {
	defer perf.Track(nil, "toolchain.Installer.Uninstall")()

	// Try to find the binary by searching
	binaryPath, err := i.FindBinaryPath(owner, repo, version)
	if err != nil {
		return fmt.Errorf("%w: tool %s/%s@%s is not installed", ErrToolNotFound, owner, repo, version)
	}

	// Get the directory containing the binary
	binaryDir := filepath.Dir(binaryPath)

	// Remove the binary file
	if err := os.Remove(binaryPath); err != nil {
		return fmt.Errorf("%w: failed to remove binary %s: %w", ErrFileOperation, binaryPath, err)
	}

	// Try to remove the directory if it's empty
	if err := os.Remove(binaryDir); err != nil {
		// It's okay if the directory is not empty or can't be removed
		log.Debug("Could not remove directory (may not be empty)", "dir", binaryDir, "error", err)
	}

	// Try to remove parent directories if they're empty
	parentDir := filepath.Dir(binaryDir)
	for {
		if err := os.Remove(parentDir); err != nil {
			// Stop when we can't remove a directory (likely not empty)
			break
		}
		parentDir = filepath.Dir(parentDir)

		// Stop if we've reached the root of the bin directory
		if parentDir == i.binDir || parentDir == "." {
			break
		}
	}

	log.Debug("Successfully uninstalled tool", "owner", owner, "repo", repo, "version", version)
	return nil
}

// FindBinaryPath searches for a binary with the given owner, repo, and version.
// The binaryName parameter is optional - pass empty string to auto-detect.
func (i *Installer) FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error) {
	defer perf.Track(nil, "toolchain.installBinaryFromGitHub")()

	// Handle "latest" keyword
	if version == "latest" {
		actualVersion, err := i.ReadLatestFile(owner, repo)
		if err != nil {
			return "", fmt.Errorf("%w: failed to read latest version for %s/%s: %w", ErrFileOperation, owner, repo, err)
		}
		version = actualVersion
	}

	// Extract binary name from variadic parameter.
	name := ""
	if len(binaryName) > 0 && binaryName[0] != "" {
		name = binaryName[0]
	}

	// Try the expected path first (binDir/owner/repo/version/binaryName)
	expectedPath := i.getBinaryPath(owner, repo, version, name)
	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	}

	// Try the alternative path structure (binDir/version/binaryName) that was used in some installations
	// Determine the binary name (use repo name as default if not provided)
	fallbackName := repo
	if name != "" {
		fallbackName = name
	}

	alternativePath := filepath.Join(i.binDir, version, fallbackName)
	if _, err := os.Stat(alternativePath); err == nil {
		return alternativePath, nil
	}

	// If neither path exists, return an error
	return "", fmt.Errorf("%w: binary not found at expected paths: %s or %s", ErrToolNotFound, expectedPath, alternativePath)
}

// CreateLatestFile creates a "latest" file that contains a pointer to the actual version.
func (i *Installer) CreateLatestFile(owner, repo, version string) error {
	defer perf.Track(nil, "toolchain.WriteSymlink")()

	// Create the latest file path
	latestDir := filepath.Join(i.binDir, owner, repo)
	if err := os.MkdirAll(latestDir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create latest directory: %w", ErrFileOperation, err)
	}

	latestFilePath := filepath.Join(latestDir, "latest")

	// Write the version to the latest file
	if err := os.WriteFile(latestFilePath, []byte(version), defaultFileWritePermissions); err != nil {
		return fmt.Errorf("%w: failed to write latest file: %w", ErrFileOperation, err)
	}

	log.Debug("Created latest file", "path", latestFilePath, "version", version)
	return nil
}

// ReadLatestFile reads the version from a "latest" file.
func (i *Installer) ReadLatestFile(owner, repo string) (string, error) {
	defer perf.Track(nil, "toolchain.Installer.ReadLatestFile")()

	latestFilePath := filepath.Join(i.binDir, owner, repo, "latest")

	data, err := os.ReadFile(latestFilePath)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read latest file: %w", ErrFileOperation, err)
	}

	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("%w: latest file is empty", ErrFileOperation)
	}

	return version, nil
}

// searchRegistryForTool searches the Aqua registry for a tool by name.
func searchRegistryForTool(toolName string) (string, string, error) {
	// Try to find the tool by searching the registry
	// This is a simplified search - in a real implementation, you'd want to
	// cache the registry contents and search more efficiently

	// For now, we'll try some common registry paths
	commonPaths := []string{
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", toolName, toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/hashicorp/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/cloudposse/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/kubernetes/kubernetes/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/registry.yaml", toolName),
	}

	client := httpClient.NewDefaultClient(
		httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
	)

	for _, path := range commonPaths {
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			// Extract owner/repo from the URL
			// path format: .../pkgs/owner/repo/registry.yaml
			parts := strings.Split(path, "/")
			if len(parts) >= 8 {
				owner := parts[len(parts)-3]
				repo := parts[len(parts)-2]
				return owner, repo, nil
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	return "", "", fmt.Errorf("%w: '%s' not found in registry", ErrToolNotFound, toolName)
}

func (i *Installer) GetResolver() ToolResolver {
	defer perf.Track(nil, "toolchain.GetResolver")()

	return i.resolver
}
