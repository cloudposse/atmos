package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gabriel-vasile/mimetype"
	"gopkg.in/yaml.v3"
)

// PackageRegistry represents the structure of a package registry YAML file
type PackageRegistry struct {
	Packages []Package `yaml:"packages"`
}

// Package represents a single package in the registry
type Package struct {
	Name         string            `yaml:"name"`
	Registry     string            `yaml:"registry"`
	Version      string            `yaml:"version"`
	Type         string            `yaml:"type"`
	RepoOwner    string            `yaml:"repo_owner"`
	RepoName     string            `yaml:"repo_name"`
	Asset        string            `yaml:"asset"`
	Format       string            `yaml:"format"`
	Files        []File            `yaml:"files"`
	Overrides    []Override        `yaml:"overrides"`
	SupportedIf  *SupportedIf      `yaml:"supported_if"`
	Replacements map[string]string `yaml:"replacements"`
}

// File represents a file to be extracted from the archive
type File struct {
	Name string `yaml:"name"`
	Src  string `yaml:"src"`
}

// Override represents platform-specific overrides
type Override struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
	Asset  string `yaml:"asset"`
	Files  []File `yaml:"files"`
}

// SupportedIf represents conditions for when a package is supported
type SupportedIf struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
}

// ToolResolver defines an interface for resolving tool names to owner/repo pairs
// This allows for mocking in tests and flexible resolution in production
type ToolResolver interface {
	Resolve(toolName string) (owner, repo string, err error)
}

// DefaultToolResolver implements ToolResolver using the existing logic
type DefaultToolResolver struct{}

func (d *DefaultToolResolver) Resolve(toolName string) (string, string, error) {
	// First, check local config aliases
	lcm := NewLocalConfigManager()
	if err := lcm.Load("tools.yaml"); err == nil {
		if alias, exists := lcm.ResolveAlias(toolName); exists {
			parts := strings.Split(alias, "/")
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
	}
	// Try to find the package in the Aqua registry
	owner, repo, err := searchRegistryForTool(toolName)
	if err == nil {
		return owner, repo, nil
	}
	return "", "", fmt.Errorf("tool '%s' not found in aliases or registry", toolName)
}

// Installer handles the installation of CLI binaries
type Installer struct {
	registryPath string
	cacheDir     string
	binDir       string
	registries   []string
	resolver     ToolResolver
}

// NewInstallerWithResolver allows injecting a custom ToolResolver (for tests)
func NewInstallerWithResolver(resolver ToolResolver) *Installer {
	homeDir, _ := os.UserHomeDir()
	cacheDir := filepath.Join(homeDir, ".cache", "installer")
	binDir := "./.tools/bin"
	registries := []string{
		"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
		"./package-registry",
	}
	return &Installer{
		registryPath: "./package-registry",
		cacheDir:     cacheDir,
		binDir:       binDir,
		registries:   registries,
		resolver:     resolver,
	}
}

// NewInstaller uses the default resolver
func NewInstaller() *Installer {
	return NewInstallerWithResolver(&DefaultToolResolver{})
}

// Install installs a package from the registry
func (i *Installer) Install(owner, repo, version string) (string, error) {
	// 1. Try local config manager first
	lcm := i.getLocalConfigManager()
	if lcm != nil {
		pkg, err := lcm.GetPackageWithVersion(owner, repo, version)
		if err == nil && pkg != nil {
			return i.installFromPackage(pkg, version)
		}
	}

	// 2. Fallback to Aqua registry
	pkg, err := i.findPackage(owner, repo, version)
	if err != nil {
		return "", fmt.Errorf("failed to get package from registry: %w", err)
	}
	return i.installFromPackage(pkg, version)
}

// Helper to handle the rest of the install logic
func (i *Installer) installFromPackage(pkg *Package, version string) (string, error) {
	assetURL, err := i.buildAssetURL(pkg, version)
	if err != nil {
		return "", fmt.Errorf("failed to build asset URL: %w", err)
	}
	log.Debug("Downloading package", "owner", pkg.RepoOwner, "repo", pkg.RepoName, "version", version, "url", assetURL)
	assetPath, err := i.downloadAsset(assetURL)
	if err != nil {
		return "", fmt.Errorf("failed to download asset: %w", err)
	}
	binaryPath, err := i.extractAndInstall(pkg, assetPath, version)
	if err != nil {
		return "", fmt.Errorf("failed to extract and install: %w", err)
	}
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}
	// Set mod time to now so install date reflects installation, not archive timestamp
	now := time.Now()
	_ = os.Chtimes(binaryPath, now, now)
	return binaryPath, nil
}

// InstallFromToolVersions installs packages specified in .tool-versions file
func (i *Installer) InstallFromToolVersions(toolVersionsPath string) error {
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
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
			log.Error("Failed to install package", "owner", owner, "repo", repo, "version", version, "error", err)
			continue
		}

		log.Debug("Successfully installed package", "owner", owner, "repo", repo, "version", version)
	}

	return nil
}

// Run executes a specific version of a package
func (i *Installer) Run(owner, repo, version string, args []string) error {
	// Find the binary path for this version
	binaryPath := i.getBinaryPath(owner, repo, version)

	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("package %s/%s@%s is not installed. Run 'toolchain install %s/%s@%s' first",
			owner, repo, version, owner, repo, version)
	}

	// Execute the binary with arguments
	return i.executeBinary(binaryPath, args)
}

// RunFromToolVersions runs a tool using the version specified in .tool-versions
func (i *Installer) RunFromToolVersions(tool string, args []string) error {
	toolVersions, err := LoadToolVersions(".tool-versions")
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	versions, exists := toolVersions.Tools[tool]
	if !exists {
		return fmt.Errorf("tool '%s' not found in .tool-versions", tool)
	}

	if len(versions) == 0 {
		return fmt.Errorf("no version specified for tool '%s' in .tool-versions", tool)
	}

	owner, repo, err := i.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("invalid tool specification: %w", err)
	}

	version := versions[0]
	return i.Run(owner, repo, version, args)
}

// findPackage searches for a package in the registry
func (i *Installer) findPackage(owner, repo, version string) (*Package, error) {
	// Search through all registries
	for _, registry := range i.registries {
		pkg, err := i.searchRegistry(registry, owner, repo, version)
		if err == nil {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("package %s/%s@%s not found in any registry", owner, repo, version)
}

// searchRegistry searches a specific registry for a package
func (i *Installer) searchRegistry(registry, owner, repo, version string) (*Package, error) {
	// For demonstration, we'll use a hardcoded package for github-comment
	// In a real implementation, you'd scan the registry

	if owner == "suzuki-shunsuke" && repo == "github-comment" {
		// Return a hardcoded package for demonstration
		return &Package{
			Name:      "github-comment",
			Type:      "github_release",
			RepoOwner: "suzuki-shunsuke",
			RepoName:  "github-comment",
			Asset:     "github-comment_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
			Files: []File{
				{
					Name: "github-comment",
					Src:  "github-comment",
				},
			},
		}, nil
	}

	// Try to fetch from remote registry
	if strings.HasPrefix(registry, "http") {
		return i.fetchFromRemoteRegistry(registry, owner, repo, version)
	}

	// Try local registry
	return i.searchLocalRegistry(registry, owner, repo, version)
}

// fetchFromRemoteRegistry fetches package definition from a remote registry
func (i *Installer) fetchFromRemoteRegistry(registryURL, owner, repo, version string) (*Package, error) {
	// Create cache directory for registry files
	registryCacheDir := filepath.Join(i.cacheDir, "registries")
	if err := os.MkdirAll(registryCacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create registry cache directory: %w", err)
	}

	// Create cache key for this registry
	hash := sha256.Sum256([]byte(registryURL))
	cacheKey := hex.EncodeToString(hash[:])
	cacheFile := filepath.Join(registryCacheDir, cacheKey+".yaml")

	// Check if we have a cached version
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		// Fetch from remote
		url := fmt.Sprintf("%s/%s/%s.yaml", registryURL, owner, repo)
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from remote registry: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch from remote registry: HTTP %d", resp.StatusCode)
		}

		// Save to cache
		file, err := os.Create(cacheFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create cache file: %w", err)
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to write cache file: %w", err)
		}
	}

	// Load from cache
	return i.loadPackageFile(cacheFile)
}

// searchLocalRegistry searches a local registry for a package
func (i *Installer) searchLocalRegistry(registryPath, owner, repo, version string) (*Package, error) {
	packageFile := filepath.Join(registryPath, owner, repo+".yaml")
	if _, err := os.Stat(packageFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("package file not found: %s", packageFile)
	}

	return i.loadPackageFile(packageFile)
}

// loadPackageFile loads a package YAML file
func (i *Installer) loadPackageFile(filePath string) (*Package, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var pkg PackageRegistry
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	// Return the first package (assuming single package per file)
	if len(pkg.Packages) > 0 {
		return &pkg.Packages[0], nil
	}

	return nil, fmt.Errorf("no packages found in %s", filePath)
}

// parseToolSpec parses a tool specification (owner/repo or just repo)
func (i *Installer) parseToolSpec(tool string) (string, string, error) {
	parts := strings.Split(tool, "/")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	} else if len(parts) == 1 {
		return i.resolver.Resolve(parts[0])
	}
	return "", "", fmt.Errorf("invalid tool specification: %s", tool)
}

// getLocalConfigManager returns a local config manager instance
func (i *Installer) getLocalConfigManager() *LocalConfigManager {
	lcm := NewLocalConfigManager()
	if err := lcm.Load("tools.yaml"); err != nil {
		log.Warn("Failed to load local config", "error", err)
		return nil
	}
	return lcm
}

// buildAssetURL constructs the download URL for the asset
func (i *Installer) buildAssetURL(pkg *Package, version string) (string, error) {
	// Use the asset template from the package
	assetTemplate := pkg.Asset
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
		RepoOwner: pkg.RepoOwner,
		RepoName:  pkg.RepoName,
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

	// Construct the full URL
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		pkg.RepoOwner, pkg.RepoName, version, assetName.String())

	return url, nil
}

// downloadAsset downloads an asset to the cache directory
func (i *Installer) downloadAsset(url string) (string, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(i.cacheDir, 0755); err != nil {
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

	// Download the file
	log.Debug("Downloading asset", "filename", filename)
	resp, err := http.Get(url)
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

// extractAndInstall extracts the binary from the asset and installs it
func (i *Installer) extractAndInstall(pkg *Package, assetPath, version string) (string, error) {
	// Create version-specific directory
	versionDir := filepath.Join(i.binDir, pkg.RepoOwner, pkg.RepoName, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create version directory: %w", err)
	}

	// Determine the binary name
	binaryName := pkg.Name
	if binaryName == "" {
		binaryName = pkg.RepoName
	}

	binaryPath := filepath.Join(versionDir, binaryName)

	// For now, just copy the file (simplified extraction)
	if err := i.simpleExtract(assetPath, binaryPath, pkg); err != nil {
		return "", fmt.Errorf("failed to extract: %w", err)
	}

	return binaryPath, nil
}

// simpleExtract is a robust extraction method using magic file type detection
func (i *Installer) simpleExtract(assetPath, binaryPath string, pkg *Package) error {
	// Detect file type using magic bytes
	mime, err := mimetype.DetectFile(assetPath)
	if err != nil {
		return fmt.Errorf("failed to detect file type: %w", err)
	}

	log.Debug("Detected file type", "mime", mime.String(), "extension", mime.Extension())

	switch {
	case mime.Is("application/zip"):
		return i.extractZip(assetPath, binaryPath, pkg)
	case mime.Is("application/x-gzip") || mime.Is("application/gzip"):
		// Check if it's a tar.gz (by extension or by magic)
		if strings.HasSuffix(assetPath, ".tar.gz") || strings.HasSuffix(assetPath, ".tgz") || mime.Is("application/x-tar") {
			return i.extractTarGz(assetPath, binaryPath, pkg)
		}
		// Otherwise, treat as a single gzip-compressed binary
		return i.extractGzip(assetPath, binaryPath)
	case mime.Is("application/x-tar"):
		return i.extractTarGz(assetPath, binaryPath, pkg)
	case mime.Is("application/octet-stream") || mime.Is("application/x-executable"):
		return i.copyFile(assetPath, binaryPath)
	default:
		// Fallback to extension-based detection
		if strings.HasSuffix(assetPath, ".zip") {
			return i.extractZip(assetPath, binaryPath, pkg)
		}
		if strings.HasSuffix(assetPath, ".tar.gz") || strings.HasSuffix(assetPath, ".tgz") {
			return i.extractTarGz(assetPath, binaryPath, pkg)
		}
		if strings.HasSuffix(assetPath, ".gz") {
			return i.extractGzip(assetPath, binaryPath)
		}
		log.Debug("Unknown file type, copying as binary", "filename", filepath.Base(assetPath))
		return i.copyFile(assetPath, binaryPath)
	}
}

// extractZip extracts a ZIP file
func (i *Installer) extractZip(zipPath, binaryPath string, pkg *Package) error {
	log.Debug("Extracting ZIP archive", "filename", filepath.Base(zipPath))

	tempDir, err := ioutil.TempDir("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("unzip", "-o", zipPath, "-d", tempDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract ZIP: %w, output: %s", err, string(output))
	}

	binaryName := pkg.Name
	if binaryName == "" {
		binaryName = pkg.RepoName
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Move the binary into place
	if err := os.Rename(found, binaryPath); err != nil {
		return fmt.Errorf("failed to move extracted binary: %w", err)
	}

	return nil
}

// extractTarGz extracts a tar.gz file
func (i *Installer) extractTarGz(tarPath, binaryPath string, pkg *Package) error {
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

	binaryName := pkg.Name
	if binaryName == "" {
		binaryName = pkg.RepoName
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Move the binary into place
	if err := os.Rename(found, binaryPath); err != nil {
		return fmt.Errorf("failed to move extracted binary: %w", err)
	}

	return nil
}

// extractGzip decompresses a single gzip-compressed binary
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

// copyFile copies a file
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

// getBinaryPath returns the path to a specific version of a binary
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

// executeBinary executes a binary with the given arguments
func (i *Installer) executeBinary(binaryPath string, args []string) error {
	// This would use os/exec to run the binary
	// For now, just print what would be executed
	log.Debug("Would execute binary", "path", binaryPath, "args", args)
	return nil
}

// Uninstall removes a previously installed package
func (i *Installer) Uninstall(owner, repo, version string) error {
	// Try to find the binary by searching
	binaryPath, err := i.findBinaryPath(owner, repo, version)
	if err != nil {
		return fmt.Errorf("package %s/%s@%s is not installed", owner, repo, version)
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

	log.Debug("Successfully uninstalled package", "owner", owner, "repo", repo, "version", version)
	return nil
}

// findBinaryPath searches for a binary with the given owner, repo, and version
func (i *Installer) findBinaryPath(owner, repo, version string) (string, error) {
	// Handle "latest" keyword
	if version == "latest" {
		actualVersion, err := i.readLatestFile(owner, repo)
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

// createLatestFile creates a "latest" file that contains a pointer to the actual version
func (i *Installer) createLatestFile(owner, repo, version string) error {
	// Create the latest file path
	latestDir := filepath.Join(i.binDir, owner, repo)
	if err := os.MkdirAll(latestDir, 0755); err != nil {
		return fmt.Errorf("failed to create latest directory: %w", err)
	}

	latestFilePath := filepath.Join(latestDir, "latest")

	// Write the version to the latest file
	if err := os.WriteFile(latestFilePath, []byte(version), 0644); err != nil {
		return fmt.Errorf("failed to write latest file: %w", err)
	}

	log.Debug("Created latest file", "path", latestFilePath, "version", version)
	return nil
}

// readLatestFile reads the version from a "latest" file
func (i *Installer) readLatestFile(owner, repo string) (string, error) {
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

// searchRegistryForTool searches the Aqua registry for a tool by name
func searchRegistryForTool(toolName string) (string, string, error) {
	// Try to find the package by searching the registry
	// This is a simplified search - in a real implementation, you'd want to
	// cache the registry contents and search more efficiently

	// For now, we'll try some common registry paths
	commonPaths := []string{
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", toolName, toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/hashicorp/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu/%s/registry.yaml", toolName),
	}

	for _, path := range commonPaths {
		resp, err := http.Get(path)
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
