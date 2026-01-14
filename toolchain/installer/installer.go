package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/homedir"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
	"github.com/cloudposse/atmos/toolchain/registry"
)

const (
	// VersionPrefix is the standard version prefix for tools.
	VersionPrefix               = "v"
	defaultFileWritePermissions = 0o644
	defaultMkdirPermissions     = 0o755
	executablePermissionMask    = 0o111 // Mask for checking if file is executable.
	maxUnixPermissions          = 0o7777
	maxDecompressedSizeMB       = 3000
	bufferSizeBytes             = 32 * 1024

	// Registry path parsing constants.
	minRegistryPathSegments = 8          // Minimum path segments for registry.yaml parsing.
	filenameKey             = "filename" // Key for filename in template replacements.

	// Log field names for consistent debugging.
	logFieldOwner   = "owner"
	logFieldRepo    = "repo"
	logFieldVersion = "version"
)

// ToolResolver defines an interface for resolving tool names to owner/repo pairs
// This allows for mocking in tests and flexible resolution in production.
type ToolResolver interface {
	Resolve(toolName string) (owner, repo string, err error)
}

// BuiltinAliases are always available and can be overridden in atmos.yaml.
// These provide convenient shortcuts for common tools.
var BuiltinAliases = map[string]string{
	"atmos": "cloudposse/atmos",
}

// DefaultToolResolver implements ToolResolver using configured aliases and registry search.
type DefaultToolResolver struct {
	AtmosConfig *schema.AtmosConfiguration
}

func (d *DefaultToolResolver) Resolve(toolName string) (string, string, error) {
	defer perf.Track(nil, "toolchain.DefaultToolResolver.Resolve")()

	// Step 1: Check if this is an alias in atmos.yaml (user-defined aliases take precedence).
	// If atmosConfig is available and has aliases configured, resolve the alias first.
	if d.AtmosConfig != nil && len(d.AtmosConfig.Toolchain.Aliases) > 0 {
		if aliasedName, found := d.AtmosConfig.Toolchain.Aliases[toolName]; found {
			toolName = aliasedName
		}
	}

	// Step 1b: Check built-in aliases (if not already resolved by user config).
	if aliasedName, found := BuiltinAliases[toolName]; found {
		toolName = aliasedName
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
		WithExplanationf("Tool '%s' not found in Aqua registry", toolName).
		WithHintf("Add an alias in atmos.yaml:\n```yaml\ntoolchain:\n  aliases:\n    %s: owner/repo\n```", toolName).
		WithHint("Or use full format: owner/repo (e.g., hashicorp/terraform)").
		WithHint("Run 'atmos toolchain registry search' to browse available tools").
		WithHint("See https://atmos.tools/cli/commands/toolchain/ for toolchain configuration").
		WithContext("tool", toolName).
		WithExitCode(2).
		Err()
}

// Installer handles the installation of CLI binaries.
type Installer struct {
	registryPath     string
	cacheDir         string
	binDir           string
	registries       []string
	resolver         ToolResolver
	configuredReg    registry.ToolRegistry // Registry loaded from atmos.yaml config.
	useConfiguredReg bool                  // Whether to use configured registry vs legacy hardcoded list.
	registryFactory  RegistryFactory       // Factory for creating Aqua registry instances.
}

// RegistryFactory creates registry instances. This allows dependency injection.
type RegistryFactory interface {
	NewAquaRegistry() registry.ToolRegistry
}

// Option is a functional option for configuring the Installer.
type Option func(*Installer)

// WithBinDir sets the binary installation directory.
func WithBinDir(binDir string) Option {
	defer perf.Track(nil, "installer.WithBinDir")()

	return func(i *Installer) {
		i.binDir = binDir
	}
}

// WithCacheDir sets the cache directory.
func WithCacheDir(cacheDir string) Option {
	defer perf.Track(nil, "installer.WithCacheDir")()

	return func(i *Installer) {
		i.cacheDir = cacheDir
	}
}

// WithResolver sets the tool resolver.
func WithResolver(resolver ToolResolver) Option {
	defer perf.Track(nil, "installer.WithResolver")()

	return func(i *Installer) {
		i.resolver = resolver
	}
}

// WithAtmosConfig sets the AtmosConfig on the default resolver for alias resolution.
// This must be called after the installer is created to update the default resolver.
func WithAtmosConfig(config *schema.AtmosConfiguration) Option {
	defer perf.Track(nil, "installer.WithAtmosConfig")()

	return func(i *Installer) {
		// If using the default resolver, update its AtmosConfig.
		if resolver, ok := i.resolver.(*DefaultToolResolver); ok {
			resolver.AtmosConfig = config
		}
	}
}

// WithConfiguredRegistry sets a pre-configured registry.
func WithConfiguredRegistry(reg registry.ToolRegistry) Option {
	defer perf.Track(nil, "installer.WithConfiguredRegistry")()

	return func(i *Installer) {
		i.configuredReg = reg
		i.useConfiguredReg = true
	}
}

// WithRegistryFactory sets the factory for creating registry instances.
func WithRegistryFactory(factory RegistryFactory) Option {
	defer perf.Track(nil, "installer.WithRegistryFactory")()

	return func(i *Installer) {
		i.registryFactory = factory
	}
}

// defaultRegistryFactory is a no-op factory that returns nil.
// The actual factory is injected from the toolchain package.
type defaultRegistryFactory struct{}

func (d *defaultRegistryFactory) NewAquaRegistry() registry.ToolRegistry {
	defer perf.Track(nil, "installer.defaultRegistryFactory.NewAquaRegistry")()

	return nil
}

// New creates a new Installer with the given options.
func New(opts ...Option) *Installer {
	defer perf.Track(nil, "installer.New")()

	// Use XDG-compliant cache directory.
	cacheDir, err := xdg.GetXDGCacheDir("toolchain", defaultMkdirPermissions)
	if err != nil || cacheDir == "" {
		// Fallback to manual construction if XDG fails.
		log.Warn("XDG cache dir unavailable, falling back to manual construction", "error", err)
		homeDir, _ := homedir.Dir()
		if homeDir == "" {
			log.Warn("Falling back to temp dir for cache")
			cacheDir = filepath.Join(os.TempDir(), ".cache", "tools-cache")
		} else {
			cacheDir = filepath.Join(homeDir, ".cache", "tools-cache")
		}
	}

	installer := &Installer{
		registryPath: "./tool-registry",
		cacheDir:     cacheDir,
		registries: []string{
			"https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs",
			"./tool-registry",
		},
		registryFactory: &defaultRegistryFactory{},
		resolver:        &DefaultToolResolver{}, // Default resolver.
	}

	// Apply options.
	for _, opt := range opts {
		opt(installer)
	}

	return installer
}

// NewInstallerWithResolver allows injecting a custom ToolResolver (for tests).
// Deprecated: Use New() with WithResolver() option instead.
func NewInstallerWithResolver(resolver ToolResolver, binDir string) *Installer {
	defer perf.Track(nil, "installer.NewInstallerWithResolver")()

	return New(
		WithResolver(resolver),
		WithBinDir(binDir),
	)
}

// Install installs a tool from the registry.
func (i *Installer) Install(owner, repo, version string) (string, error) {
	defer perf.Track(nil, "installer.Install")()

	// Get tool from registry
	tool, err := i.FindTool(owner, repo, version)
	if err != nil {
		return "", err // Error already enriched in findTool
	}
	return i.installFromTool(tool, version)
}

// Helper to handle the rest of the install logic.
func (i *Installer) installFromTool(tool *registry.Tool, version string) (string, error) {
	// Set version on tool so extraction functions can use it for template expansion.
	tool.Version = version

	// Apply platform-specific overrides before building the asset URL.
	ApplyPlatformOverrides(tool)

	assetURL, err := i.BuildAssetURL(tool, version)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrInvalidToolSpec, err)
	}
	log.Debug("Downloading tool", "owner", tool.RepoOwner, "repo", tool.RepoName, logFieldVersion, version, "url", assetURL)

	assetPath, err := i.downloadAssetWithVersionFallback(tool, version, assetURL)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrHTTPRequest, err)
	}
	binaryPath, err := i.extractAndInstall(tool, assetPath, version)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrFileOperation, err)
	}
	if err := os.Chmod(binaryPath, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to make binary executable: %w", ErrFileOperation, err)
	}
	// Set mod time to now so install date reflects installation, not archive timestamp
	now := time.Now()
	_ = os.Chtimes(binaryPath, now, now)
	return binaryPath, nil
}

// FindTool searches for a tool in the registry.
func (i *Installer) FindTool(owner, repo, version string) (*registry.Tool, error) {
	defer perf.Track(nil, "installer.FindTool")()

	log.Debug("Finding tool in registries",
		logFieldOwner, owner,
		logFieldRepo, repo,
		logFieldVersion, version,
		"useConfiguredReg", i.useConfiguredReg,
		"hasConfiguredReg", i.configuredReg != nil,
		"legacyRegistryCount", len(i.registries))

	// Use configured registries from atmos.yaml if available.
	if tool, found := i.findToolInConfiguredRegistry(owner, repo, version); found {
		return tool, nil
	}

	// Fallback: Search through legacy hardcoded registries.
	if tool, found := i.findToolInLegacyRegistries(owner, repo, version); found {
		return tool, nil
	}

	// Tool not found, return error with context.
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

// findToolInConfiguredRegistry searches for a tool in configured registries.
func (i *Installer) findToolInConfiguredRegistry(owner, repo, version string) (*registry.Tool, bool) {
	if !i.useConfiguredReg || i.configuredReg == nil {
		log.Debug("No configured registries available, using legacy hardcoded registries",
			"useConfiguredReg", i.useConfiguredReg,
			"hasConfiguredReg", i.configuredReg != nil)
		return nil, false
	}

	log.Debug("Searching configured registries from atmos.yaml")
	tool, err := i.configuredReg.GetToolWithVersion(owner, repo, version)
	if err == nil {
		log.Debug("Tool found in configured registry",
			logFieldOwner, owner,
			logFieldRepo, repo,
			logFieldVersion, version,
			"toolRegistry", tool.Registry,
			"toolAsset", tool.Asset)
		// Ensure RepoOwner and RepoName are set correctly.
		tool.RepoOwner = owner
		tool.RepoName = repo
		return tool, true
	}

	log.Debug("Tool not found in configured registries",
		logFieldOwner, owner,
		logFieldRepo, repo,
		logFieldVersion, version,
		"error", err)
	return nil, false
}

// findToolInLegacyRegistries searches through legacy hardcoded registries.
func (i *Installer) findToolInLegacyRegistries(owner, repo, version string) (*registry.Tool, bool) {
	log.Debug("Falling back to legacy hardcoded registries", "count", len(i.registries))
	for _, reg := range i.registries {
		log.Debug("Searching legacy registry", "registry", reg)
		tool, err := i.searchRegistry(reg, owner, repo, version)
		if err == nil {
			log.Debug("Tool found in legacy registry", "registry", reg)
			return tool, true
		}
		log.Debug("Tool not found in legacy registry", "registry", reg, "error", err)
	}
	return nil, false
}

// searchRegistry searches a specific registry for a tool.
// Version is required to apply version-specific overrides from the registry.
func (i *Installer) searchRegistry(reg, owner, repo, version string) (*registry.Tool, error) {
	// Try to fetch from Aqua registry for remote registries.
	if strings.HasPrefix(reg, "http") {
		// Use the injected registry factory.
		if i.registryFactory == nil {
			return nil, fmt.Errorf("%w: registry factory not configured", ErrInvalidToolSpec)
		}
		ar := i.registryFactory.NewAquaRegistry()
		if ar == nil {
			return nil, fmt.Errorf("%w: failed to create Aqua registry", ErrInvalidToolSpec)
		}
		tool, err := ar.GetToolWithVersion(owner, repo, version)
		if err != nil {
			return nil, err
		}
		// Ensure RepoOwner and RepoName are set correctly.
		tool.RepoOwner = owner
		tool.RepoName = repo
		return tool, nil
	}

	// Try local registry.
	return i.searchLocalRegistry(reg, owner, repo)
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

// ParseToolSpec parses a tool specification (owner/repo or just repo).
func (i *Installer) ParseToolSpec(tool string) (string, string, error) {
	defer perf.Track(nil, "installer.ParseToolSpec")()

	parts := strings.Split(tool, "/")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	} else if len(parts) == 1 {
		return i.resolver.Resolve(parts[0])
	}
	return "", "", fmt.Errorf("%w: invalid tool specification: %s", ErrInvalidToolSpec, tool)
}

// extractAndInstall extracts the binary from the asset and installs it.
func (i *Installer) extractAndInstall(tool *registry.Tool, assetPath, version string) (string, error) {
	// Create version-specific directory
	versionDir := filepath.Join(i.binDir, tool.RepoOwner, tool.RepoName, version)
	if err := os.MkdirAll(versionDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create version directory: %w", ErrFileOperation, err)
	}

	// Determine the binary name using shared resolution logic.
	binaryName := resolveBinaryName(tool)

	binaryPath := filepath.Join(versionDir, binaryName)

	// For now, just copy the file (simplified extraction)
	if err := i.simpleExtract(assetPath, binaryPath, tool); err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrFileOperation, err)
	}

	return binaryPath, nil
}

// GetBinDir returns the binary installation directory.
func (i *Installer) GetBinDir() string {
	defer perf.Track(nil, "installer.Installer.GetBinDir")()

	return i.binDir
}

// GetBinaryPath returns the path to a specific version of a binary.
// If binaryName is provided and non-empty, it will be used directly.
// Otherwise, it will search the version directory for an executable file,
// falling back to using the repo name as the binary name.
func (i *Installer) GetBinaryPath(owner, repo, version, binaryName string) string {
	defer perf.Track(nil, "installer.Installer.GetBinaryPath")()

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
			if err != nil {
				continue
			}
			// On Unix, check executable permission bits.
			// On Windows, check for .exe extension (permission bits don't apply).
			isExec := info.Mode()&executablePermissionMask != 0
			if runtime.GOOS == "windows" {
				isExec = strings.HasSuffix(strings.ToLower(entry.Name()), ".exe")
			}
			if isExec {
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

	log.Debug("Successfully uninstalled tool", logFieldOwner, owner, logFieldRepo, repo, "version", version)
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
	expectedPath := i.GetBinaryPath(owner, repo, version, name)
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
