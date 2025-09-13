package toolchain

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/gabriel-vasile/mimetype"
	"gopkg.in/yaml.v3"
)

// ToolResolver defines an interface for resolving tool names to owner/repo pairs
// This allows for mocking in tests and flexible resolution in production.
type ToolResolver interface {
	Resolve(toolName string) (owner, repo string, err error)
}

// DefaultToolResolver implements ToolResolver using the existing logic.
type DefaultToolResolver struct{}

func (d *DefaultToolResolver) Resolve(toolName string) (string, string, error) {
	// First, check local config aliases
	lcm := NewLocalConfigManager()
	if err := lcm.Load(GetToolsConfigFilePath()); err == nil {
		if alias, exists := lcm.ResolveAlias(toolName); exists {
			parts := strings.Split(alias, "/")
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
	}
	// Try to find the tool in the Aqua registry
	owner, repo, err := searchRegistryForTool(toolName)
	if err == nil {
		return owner, repo, nil
	}
	return "", "", fmt.Errorf("tool '%s' not found in local aliases or Aqua registry", toolName)
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warn("Falling back to temp dir for cache.", "error", err)
		homeDir = os.TempDir()
	}
	cacheDir := filepath.Join(homeDir, ".cache", "tools-cache")
	binDir := filepath.Join(GetToolsDirPath(), "bin")
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

// NewInstaller uses the default resolver.
func NewInstaller() *Installer {
	return NewInstallerWithResolver(&DefaultToolResolver{})
}

// Install installs a tool from the registry.
func (i *Installer) Install(owner, repo, version string) (string, error) {
	// 1. Try local config manager first
	lcm := i.getLocalConfigManager()
	if lcm != nil {
		tool, err := lcm.GetToolWithVersion(owner, repo, version)
		if err == nil && tool != nil {
			return i.installFromTool(tool, version)
		}
	}

	// 2. Fallback to Aqua registry
	tool, err := i.findTool(owner, repo, version)
	if err != nil {
		return "", fmt.Errorf("failed to get tool from registry: %w", err)
	}
	return i.installFromTool(tool, version)
}

// Helper to handle the rest of the install logic.
func (i *Installer) installFromTool(tool *Tool, version string) (string, error) {
	assetURL, err := i.buildAssetURL(tool, version)
	if err != nil {
		return "", fmt.Errorf("failed to build asset URL: %w", err)
	}
	log.Debug("Downloading tool", "owner", tool.RepoOwner, "repo", tool.RepoName, "version", version, "url", assetURL)

	assetPath, err := i.downloadAssetWithVersionFallback(tool, version, assetURL)
	if err != nil {
		return "", fmt.Errorf("failed to download asset: %w", err)
	}
	binaryPath, err := i.extractAndInstall(tool, assetPath, version)
	if err != nil {
		return "", fmt.Errorf("failed to extract and install: %w", err)
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}
	// Set mod time to now so install date reflects installation, not archive timestamp
	now := time.Now()
	_ = os.Chtimes(binaryPath, now, now)
	return binaryPath, nil
}

// InstallFromToolVersions installs tools specified in tool-versions file.
func (i *Installer) InstallFromToolVersions(toolVersionsPath string) error {
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load tool-versions file: %w", err)
	}

	for tool, versions := range toolVersions.Tools {
		// Parse tool specification (owner/repo@version or just repo@version)
		owner, repo, err := i.parseToolSpec(tool)
		if err != nil {
			log.Warn("Skipping invalid tool specification", "tool", tool)
			continue
		}

		// If no version is specified, try to get the latest non-prerelease version
		if len(versions) == 0 {
			log.Warn("No version specified for tool, skipping", "tool", tool)
			continue
		}
		version := versions[0]
		log.Debug("Using version", "tool", tool, "version", version)

		log.Debug("Installing from tool-versions", "owner", owner, "repo", repo, "version", version)

		_, err = i.Install(owner, repo, version)
		if err != nil {
			log.Error("Failed to install tool", "owner", owner, "repo", repo, "version", version, "error", err)
			continue
		}

		log.Debug("Successfully installed tool", "owner", owner, "repo", repo, "version", version)
	}

	return nil
}

// Run executes a specific version of a tool.
func (i *Installer) Run(owner, repo, version string, args []string) error {
	// Find the binary path for this version
	binaryPath := i.getBinaryPath(owner, repo, version)

	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("tool %s/%s@%s is not installed. Run 'toolchain install %s/%s@%s' first",
			owner, repo, version, owner, repo, version)
	}

	// Execute the binary with arguments
	return i.executeBinary(binaryPath, args)
}

// RunFromToolVersions runs a tool using the version specified in tool-versions file.
func (i *Installer) RunFromToolVersions(tool string, args []string) error {
	toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
	if err != nil {
		return fmt.Errorf("failed to load tool-versions file: %w", err)
	}

	versions, exists := toolVersions.Tools[tool]
	if !exists {
		return fmt.Errorf("tool '%s' not found in tool-versions file", tool)
	}

	if len(versions) == 0 {
		return fmt.Errorf("no version specified for tool '%s' in tool-versions file", tool)
	}

	owner, repo, err := i.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("invalid tool specification: %w", err)
	}

	version := versions[0]
	return i.Run(owner, repo, version, args)
}

// findTool searches for a tool in the registry.
func (i *Installer) findTool(owner, repo, version string) (*Tool, error) {
	// First, try to find the tool in local configuration
	lcm := i.getLocalConfigManager()
	if lcm != nil {
		tool, err := lcm.GetToolWithVersion(owner, repo, version)
		if err == nil {
			return tool, nil
		}
	}

	// Search through all registries
	for _, registry := range i.registries {
		tool, err := i.searchRegistry(registry, owner, repo)
		if err == nil {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("tool %s/%s@%s not found in any registry", owner, repo, version)
}

// searchRegistry searches a specific registry for a tool.
func (i *Installer) searchRegistry(registry, owner, repo string) (*Tool, error) {
	// Try to fetch from Aqua registry for remote registries
	if strings.HasPrefix(registry, "http") {
		// Use the Aqua registry implementation
		ar := NewAquaRegistry()
		tool, err := ar.GetTool(owner, repo)
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
func (i *Installer) searchLocalRegistry(registryPath, owner, repo string) (*Tool, error) {
	toolFile := filepath.Join(registryPath, owner, repo+".yaml")
	if _, err := os.Stat(toolFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("tool file not found: %s", toolFile)
	}

	return i.loadToolFile(toolFile)
}

// loadToolFile loads a tool YAML file.
func (i *Installer) loadToolFile(filePath string) (*Tool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var toolToolRegistry ToolRegistry
	if err := yaml.Unmarshal(data, &toolToolRegistry); err != nil {
		return nil, err
	}

	// Return the first tool (assuming single tool per file)
	if len(toolToolRegistry.Tools) > 0 {
		return &toolToolRegistry.Tools[0], nil
	}

	return nil, fmt.Errorf("no tools found in %s", filePath)
}

// parseToolSpec parses a tool specification (owner/repo or just repo).
func (i *Installer) parseToolSpec(tool string) (string, string, error) {
	parts := strings.Split(tool, "/")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	} else if len(parts) == 1 {
		return i.resolver.Resolve(parts[0])
	}
	return "", "", fmt.Errorf("invalid tool specification: %s", tool)
}

// getLocalConfigManager returns a local config manager instance.
func (i *Installer) getLocalConfigManager() *LocalConfigManager {
	lcm := NewLocalConfigManager()
	if err := lcm.Load(GetToolsConfigFilePath()); err != nil {
		log.Warn("Failed to load local config", "error", err)
		return nil
	}
	return lcm
}

// buildAssetURL constructs the download URL for the asset.
func (i *Installer) buildAssetURL(tool *Tool, version string) (string, error) {
	// Handle different tool types
	switch tool.Type {
	case "http":
		// For HTTP type, the Asset field contains the full URL template
		if tool.Asset == "" {
			return "", fmt.Errorf("invalid tool configuration: Asset URL template is required for HTTP type tools")
		}

		// Remove 'v' prefix from version for asset naming
		versionForAsset := version
		if strings.HasPrefix(versionForAsset, "v") {
			versionForAsset = versionForAsset[1:]
		}

		// Create template data
		data := struct {
			Version   string
			OS        string
			Arch      string
			RepoOwner string
			RepoName  string
		}{
			Version:   versionForAsset,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			RepoOwner: tool.RepoOwner,
			RepoName:  tool.RepoName,
		}

		// Register custom template functions
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

		tmpl, err := template.New("asset").Funcs(funcMap).Parse(tool.Asset)
		if err != nil {
			return "", fmt.Errorf("failed to parse asset template: %w", err)
		}

		var url strings.Builder
		if err := tmpl.Execute(&url, data); err != nil {
			return "", fmt.Errorf("failed to execute asset template: %w", err)
		}

		return url.String(), nil

	case "github_release":
		// For GitHub releases, validate that RepoOwner and RepoName are set
		if tool.RepoOwner == "" || tool.RepoName == "" {
			return "", fmt.Errorf("invalid tool configuration: RepoOwner and RepoName must be set for github_release type (got RepoOwner=%q, RepoName=%q)", tool.RepoOwner, tool.RepoName)
		}

		// Use the asset template from the tool
		assetTemplate := tool.Asset
		if assetTemplate == "" {
			// Fallback to a common pattern
			assetTemplate = "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
		}

		// Remove 'v' prefix from version for asset naming
		versionForAsset := version
		if strings.HasPrefix(versionForAsset, "v") {
			versionForAsset = versionForAsset[1:]
		}

		// Create template data
		data := struct {
			Version   string
			OS        string
			Arch      string
			RepoOwner string
			RepoName  string
		}{
			Version:   versionForAsset,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			RepoOwner: tool.RepoOwner,
			RepoName:  tool.RepoName,
		}

		// Register custom template functions
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

		tmpl, err := template.New("asset").Funcs(funcMap).Parse(assetTemplate)
		if err != nil {
			return "", fmt.Errorf("failed to parse asset template: %w", err)
		}

		var assetName strings.Builder
		if err := tmpl.Execute(&assetName, data); err != nil {
			return "", fmt.Errorf("failed to execute asset template: %w", err)
		}

		// Construct the full GitHub release URL
		url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
			tool.RepoOwner, tool.RepoName, version, assetName.String())

		return url, nil

	default:
		return "", fmt.Errorf("unsupported tool type: %s", tool.Type)
	}
}

// downloadAsset downloads an asset to the cache directory.
func (i *Installer) downloadAsset(url string) (string, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(i.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
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
	client := NewDefaultHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download asset: HTTP %d", resp.StatusCode)
	}

	// Create the file
	file, err := os.Create(cachePath)
	if err != nil {
		return "", fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	// Copy the response body to the file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write cache file: %w", err)
	}

	return cachePath, nil
}

// downloadAssetWithVersionFallback tries the asset URL as-is, then with 'v' prefix or without, if 404.
func (i *Installer) downloadAssetWithVersionFallback(tool *Tool, version, assetURL string) (string, error) {
	assetPath, err := i.downloadAsset(assetURL)
	if err == nil {
		return assetPath, nil
	}
	if !isHTTP404(err) {
		return "", err
	}
	// Try fallback with or without 'v'
	var fallbackVersion string
	if strings.HasPrefix(version, "v") {
		fallbackVersion = strings.TrimPrefix(version, "v")
	} else {
		fallbackVersion = "v" + version
	}
	if fallbackVersion == version {
		return "", err // nothing to try
	}
	fallbackURL, buildErr := i.buildAssetURL(tool, fallbackVersion)
	if buildErr != nil {
		return "", fmt.Errorf("failed to build fallback asset URL: %w", buildErr)
	}
	log.Warn("Asset 404, trying fallback version", "original", assetURL, "fallback", fallbackURL)
	assetPath, err = i.downloadAsset(fallbackURL)
	if err == nil {
		return assetPath, nil
	}
	return "", fmt.Errorf("failed to download asset: tried %s and %s: %w", assetURL, fallbackURL, err)
}

// isHTTP404 returns true if the error is a 404 from downloadAsset.
func isHTTP404(err error) bool {
	return strings.Contains(err.Error(), "HTTP 404")
}

// extractAndInstall extracts the binary from the asset and installs it.
func (i *Installer) extractAndInstall(tool *Tool, assetPath, version string) (string, error) {
	// Create version-specific directory
	versionDir := filepath.Join(i.binDir, tool.RepoOwner, tool.RepoName, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create version directory: %w", err)
	}

	// Determine the binary name
	binaryName := tool.Name
	if binaryName == "" {
		binaryName = tool.RepoName
	}

	binaryPath := filepath.Join(versionDir, binaryName)

	// For now, just copy the file (simplified extraction)
	if err := i.simpleExtract(assetPath, binaryPath, tool); err != nil {
		return "", fmt.Errorf("failed to extract: %w", err)
	}

	return binaryPath, nil
}

// simpleExtract is a robust extraction method using magic file type detection.
func (i *Installer) simpleExtract(assetPath, binaryPath string, tool *Tool) error {
	// Detect file type using magic bytes
	mime, err := mimetype.DetectFile(assetPath)
	if err != nil {
		return fmt.Errorf("failed to detect file type: %w", err)
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
func (i *Installer) extractZip(zipPath, binaryPath string, tool *Tool) error {
	log.Debug("Extracting ZIP archive", "filename", filepath.Base(zipPath))

	tempDir, err := ioutil.TempDir("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("unzip", "-o", zipPath, "-d", tempDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		var exitCode int
		// Case 1: Command ran but failed (non-zero exit)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
			log.Debugf("Command failed with exit code %d\n", exitCode)
		} else {
			// Case 2: Command did NOT run at all (e.g., not found)
			log.Debugf("Command execution failed: %v\n", err)
		}
		return fmt.Errorf("failed to extract ZIP: %w, output: %s", err, string(output))
	}

	binaryName := tool.Name
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
		return fmt.Errorf("failed to search extracted files: %w", err)
	}
	if found == "" {
		return fmt.Errorf("binary %s not found in extracted archive", binaryName)
	}

	// Ensure the destination directory exists
	dir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Move the binary into place
	if err := os.Rename(found, binaryPath); err != nil {
		return fmt.Errorf("failed to move extracted binary: %w", err)
	}

	return nil
}

// extractTarGz extracts a tar.gz file.
func (i *Installer) extractTarGz(tarPath, binaryPath string, tool *Tool) error {
	log.Debug("Extracting tar.gz archive", "filename", filepath.Base(tarPath))

	tempDir, err := ioutil.TempDir("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("tar", "-xzf", tarPath, "-C", tempDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract tar.gz: %w, output: %s", err, string(output))
	}

	binaryName := tool.Name
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
		return fmt.Errorf("failed to search extracted files: %w", err)
	}
	if found == "" {
		return fmt.Errorf("binary %s not found in extracted archive", binaryName)
	}

	// Ensure the destination directory exists
	dir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Move the binary into place
	if err := os.Rename(found, binaryPath); err != nil {
		return fmt.Errorf("failed to move extracted binary: %w", err)
	}

	return nil
}

// extractGzip decompresses a single gzip-compressed binary.
func (i *Installer) extractGzip(gzPath, binaryPath string) error {
	log.Debug("Decompressing gzip binary", "filename", filepath.Base(gzPath))

	in, err := os.Open(gzPath)
	if err != nil {
		return fmt.Errorf("failed to open gzip file: %w", err)
	}
	defer in.Close()

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	out, err := os.Create(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, gzr); err != nil {
		return fmt.Errorf("failed to decompress gzip: %w", err)
	}

	return nil
}

// copyFile copies a file.
func (i *Installer) copyFile(src, dst string) error {
	log.Debug("Copying binary", "src", filepath.Base(src), "dst", filepath.Base(dst))

	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// getBinaryPath returns the path to a specific version of a binary.
func (i *Installer) getBinaryPath(owner, repo, version string) string {
	// Determine the binary name (use repo name as default)
	binaryName := repo

	// Try to get binary name from configuration
	if lcm := i.getLocalConfigManager(); lcm != nil {
		if toolConfig, exists := lcm.GetToolConfig(fmt.Sprintf("%s/%s", owner, repo)); exists {
			if toolConfig.BinaryName != "" {
				binaryName = toolConfig.BinaryName
			}
		}
	}

	return filepath.Join(i.binDir, owner, repo, version, binaryName)
}

// executeBinary executes a binary with the given arguments.
func (i *Installer) executeBinary(binaryPath string, args []string) error {
	// This would use os/exec to run the binary
	// For now, just print what would be executed
	log.Debug("Would execute binary", "path", binaryPath, "args", args)
	return nil
}

// Uninstall removes a previously installed tool.
func (i *Installer) Uninstall(owner, repo, version string) error {
	// Try to find the binary by searching
	binaryPath, err := i.FindBinaryPath(owner, repo, version)
	if err != nil {
		return fmt.Errorf("tool %s/%s@%s is not installed", owner, repo, version)
	}

	// Get the directory containing the binary
	binaryDir := filepath.Dir(binaryPath)

	// Remove the binary file
	if err := os.Remove(binaryPath); err != nil {
		return fmt.Errorf("failed to remove binary %s: %w", binaryPath, err)
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
func (i *Installer) FindBinaryPath(owner, repo, version string) (string, error) {
	// Handle "latest" keyword
	if version == "latest" {
		actualVersion, err := i.ReadLatestFile(owner, repo)
		if err != nil {
			return "", fmt.Errorf("failed to read latest version for %s/%s: %w", owner, repo, err)
		}
		version = actualVersion
	}

	// Try the expected path first (binDir/owner/repo/version/binaryName)
	expectedPath := i.getBinaryPath(owner, repo, version)
	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	}

	// Try the alternative path structure (binDir/version/binaryName) that was used in some installations
	// Determine the binary name (use repo name as default)
	binaryName := repo

	// Try to get binary name from configuration
	if lcm := i.getLocalConfigManager(); lcm != nil {
		if toolConfig, exists := lcm.GetToolConfig(fmt.Sprintf("%s/%s", owner, repo)); exists {
			if toolConfig.BinaryName != "" {
				binaryName = toolConfig.BinaryName
			}
		}
	}

	alternativePath := filepath.Join(i.binDir, version, binaryName)
	if _, err := os.Stat(alternativePath); err == nil {
		return alternativePath, nil
	}

	// If neither path exists, return an error
	return "", fmt.Errorf("binary not found at expected paths: %s or %s", expectedPath, alternativePath)
}

// CreateLatestFile creates a "latest" file that contains a pointer to the actual version.
func (i *Installer) CreateLatestFile(owner, repo, version string) error {
	// Create the latest file path
	latestDir := filepath.Join(i.binDir, owner, repo)
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		return fmt.Errorf("failed to create latest directory: %w", err)
	}

	latestFilePath := filepath.Join(latestDir, "latest")

	// Write the version to the latest file
	if err := os.WriteFile(latestFilePath, []byte(version), 0o644); err != nil {
		return fmt.Errorf("failed to write latest file: %w", err)
	}

	log.Debug("Created latest file", "path", latestFilePath, "version", version)
	return nil
}

// ReadLatestFile reads the version from a "latest" file.
func (i *Installer) ReadLatestFile(owner, repo string) (string, error) {
	latestFilePath := filepath.Join(i.binDir, owner, repo, "latest")

	data, err := os.ReadFile(latestFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read latest file: %w", err)
	}

	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("latest file is empty")
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
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/kubernetes/kubernetes/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/registry.yaml", toolName),
	}

	client := NewDefaultHTTPClient()
	for _, path := range commonPaths {
		resp, err := client.Get(path)
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

	return "", "", fmt.Errorf("tool '%s' not found in registry", toolName)
}

func (i *Installer) GetResolver() ToolResolver {
	return i.resolver
}
