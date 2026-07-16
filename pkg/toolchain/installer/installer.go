package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/filelock"

	errUtils "github.com/cloudposse/atmos/errors"
	github "github.com/cloudposse/atmos/pkg/github"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
	"github.com/cloudposse/atmos/pkg/xdg"
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
	// Symlink targets read from an archive are bounded to maxSymlinkTargetBytes.
	// Real link targets are short filesystem paths; a huge payload is malformed
	// or hostile and would otherwise let a crafted archive drive a large
	// allocation.
	maxSymlinkTargetBytes = 4096

	// Registry path parsing constants.
	filenameKey = "filename" // Key for filename in template replacements.

	// Log field names for consistent debugging.
	logFieldOwner   = "owner"
	logFieldRepo    = "repo"
	logFieldVersion = "version"

	// Windows constants.
	windowsExeExt = ".exe"

	// Fallback cosign verifier bootstrap version for transient GitHub latest-release lookup failures.
	// renovate: datasource=github-releases depName=sigstore/cosign.
	defaultCosignVerifierVersion = "v3.0.6"
	// LegacyCosignVerifierVersion is retained for release metadata that supplies
	// a certificate and detached signature instead of a Sigstore bundle. Cosign
	// v3 deprecates those flags and its Rekor lookup can reject otherwise-valid
	// legacy evidence; v2.6.1 verifies that evidence without the deprecated
	// path. New bundle-based metadata continues to use the current verifier.
	// renovate: datasource=github-releases depName=sigstore/cosign.
	legacyCosignVerifierVersion = "v2.6.1"
)

// EnsureWindowsExeExtension appends .exe to the binary name on Windows if not already present.
// This follows Aqua's behavior where executables need the .exe extension on Windows
// to be found by os/exec.LookPath.
func EnsureWindowsExeExtension(binaryName string) string {
	defer perf.Track(nil, "installer.EnsureWindowsExeExtension")()

	return ensureWindowsExeExtensionForOS(binaryName, runtime.GOOS)
}

func ensureWindowsExeExtensionForOS(binaryName, goos string) string {
	if goos == "windows" && !strings.HasSuffix(strings.ToLower(binaryName), windowsExeExt) {
		return binaryName + windowsExeExt
	}
	return binaryName
}

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

var defaultRegistry = registry.DefaultRegistry

type shortNameResolver interface {
	ResolveShortName(string) (string, string, error)
}

// DefaultToolResolver implements ToolResolver using configured aliases and registry search.
type DefaultToolResolver struct {
	AtmosConfig *schema.AtmosConfiguration
}

func defaultShortNameResolver() (shortNameResolver, bool) {
	reg := defaultRegistry()
	if reg == nil {
		return nil, false
	}
	resolver, ok := reg.(shortNameResolver)
	return resolver, ok
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

	// Step 3: Consult the default registry's short-name resolver (aqua-style).
	// Aqua itself has no runtime short-name resolution — `aqua g` is the upstream
	// discovery flow — so atmos provides this UX by searching the cached registry
	// index for a package whose binary name matches. The type assertion keeps
	// short-name resolution aqua-specific (matches upstream's design where short
	// names are a discovery concern, not a registry-protocol one).
	if resolver, ok := defaultShortNameResolver(); ok {
		if owner, repo, err := resolver.ResolveShortName(toolName); err == nil {
			return owner, repo, nil
		} else if !errors.Is(err, registry.ErrToolNotFound) {
			return "", "", err
		}
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
	registryPath       string
	cacheDir           string
	binDir             string
	registries         []string
	resolver           ToolResolver
	configuredReg      registry.ToolRegistry // Registry loaded from atmos.yaml config.
	useConfiguredReg   bool                  // Whether to use configured registry vs builtin registry list.
	registryFactory    RegistryFactory       // Factory for creating Aqua registry instances.
	verificationPolicy verification.Policy
	useLockFile        bool
	lockFilePath       string
	downloadProgress   func(downloaded, total int64)
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

// WithDownloadProgress reports bytes received while an asset is downloaded.
// A total value below zero means the server did not provide Content-Length.
func WithDownloadProgress(progress func(downloaded, total int64)) Option {
	defer perf.Track(nil, "installer.WithDownloadProgress")()

	return func(i *Installer) {
		i.downloadProgress = progress
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
		} else {
			log.Debug("WithAtmosConfig skipped: resolver is not DefaultToolResolver")
		}
		if config != nil {
			i.verificationPolicy = verification.PolicyFromConfig(config.Toolchain.Verification)
			i.useLockFile = config.Toolchain.UseLockFile
			i.lockFilePath = resolveLockFilePath(config)
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
		registryFactory:    &defaultRegistryFactory{},
		resolver:           &DefaultToolResolver{}, // Default resolver.
		verificationPolicy: verification.PolicyFromConfig(nil),
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

	// The complete check/extract/replace transaction for one installed version
	// must be exclusive across Atmos processes. The stable sibling lock survives
	// the atomic replacements performed by the extractor.
	versionDir := filepath.Join(i.binDir, owner, repo, version)
	if err := os.MkdirAll(filepath.Dir(versionDir), defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create installation parent directory: %w", ErrFileOperation, err)
	}
	lock := filelock.New(versionDir + ".lock")
	var binaryPath string
	err := lock.WithExclusive(context.Background(), func() error {
		// Get tool from registry while the target is protected: registry metadata
		// can choose different entrypoints for the same installed version.
		tool, findErr := i.FindTool(owner, repo, version)
		if findErr != nil {
			return findErr
		}
		var installErr error
		binaryPath, installErr = i.installFromTool(tool, version)
		return installErr
	})
	if err != nil {
		return "", err
	}
	return binaryPath, nil
}

// Helper to handle the rest of the install logic.
func (i *Installer) installFromTool(tool *registry.Tool, version string) (string, error) {
	// Set version on tool so extraction functions can use it for template expansion.
	tool.Version = version

	// Apply platform-specific overrides before building the asset URL.
	ApplyPlatformOverrides(tool)

	// Pre-flight platform check: verify the tool supports the current platform.
	// This provides a better user experience than waiting for a 404 error.
	if platformErr := CheckPlatformSupport(tool); platformErr != nil {
		return "", buildPlatformNotSupportedError(platformErr)
	}

	assetURL, err := i.BuildAssetURL(tool, version)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrInvalidToolSpec, err)
	}
	log.Debug("Downloading tool", "owner", tool.RepoOwner, "repo", tool.RepoName, logFieldVersion, version, "url", assetURL)

	assetPath, effectiveAssetURL, effectiveVersion, err := i.downloadAssetWithVersionFallback(tool, version, assetURL)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrHTTPRequest, err)
	}
	// Render files[].src and archive paths using the version that actually
	// downloaded. Some tools (e.g. nodejs) publish under a "v"-prefixed path, so a
	// bare "24.18.0" pin resolves to "v24.18.0" via the download fallback; the
	// extracted archive's top-level directory carries the same prefix. The version
	// DIRECTORY on disk still uses the originally requested `version` for
	// .tool-versions / lookup consistency.
	tool.Version = effectiveVersion

	verificationResult, err := i.verifyDownloadedAsset(tool, version, effectiveAssetURL, assetPath)
	if err != nil {
		_ = os.Remove(assetPath) // #nosec G703 -- assetPath is the installer-created cache file for the downloaded asset.
		return "", err
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
	if err := i.updateLockFile(tool, version, effectiveAssetURL, verificationResult); err != nil {
		return "", err
	}
	return binaryPath, nil
}

func (i *Installer) verifyDownloadedAsset(tool *registry.Tool, version, assetURL, assetPath string) (*verification.Result, error) {
	// Attach a GitHub token when available so checksum/signature/SLSA sidecar fetches from
	// GitHub release assets (verification.HTTPDownloader's default is an unauthenticated
	// http.DefaultClient) get the same rate-limit headroom as the asset download itself
	// (downloadToCacheOnce, a few functions away in this package, already does this).
	verifier := verification.Verifier{
		Downloader: verification.HTTPDownloader{
			Client: httpClient.NewGitHubAuthenticatedHTTPClient(github.GetGitHubToken()),
		},
	}
	result, err := verifier.Verify(context.Background(), verification.Request{
		Tool:      tool,
		Version:   version,
		AssetURL:  assetURL,
		AssetPath: assetPath,
		Policy:    i.verificationPolicy,
		Runner:    verifierCommandRunner{installer: i, policy: i.verificationPolicy},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to verify downloaded asset: %w", err)
	}
	return result, nil
}

type verifierCommandRunner struct {
	installer *Installer
	policy    verification.Policy
}

func (r verifierCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	defer perf.Track(nil, "installer.verifierCommandRunner.Run")()

	legacyVersion := legacyVerifierVersion(name, args)
	// Do not route legacy certificate/signature verification through a
	// user- or Atmos-provided Cosign v3 found on PATH: v3 deprecated this path
	// and its Rekor client can reject valid legacy evidence. Bootstrap the
	// compatible v2 verifier below instead.
	if path, err := exec.LookPath(name); err == nil && legacyVersion == "" {
		return runVerifierCommand(ctx, path, args...)
	}
	if r.policy.VerifierInstall != verification.VerifierInstallAuto {
		return verification.ExecRunner{}.Run(ctx, name, args...)
	}
	owner, repo, ok := verifierTool(name)
	if !ok {
		return verification.ExecRunner{}.Run(ctx, name, args...)
	}
	bootstrap := *r.installer
	bootstrap.verificationPolicy = verification.Policy{
		Checksums:       verification.PolicyWhenAvailable,
		Signatures:      verification.PolicyDisabled, // Avoid circularity: verifying cosign's signature would itself need cosign.
		VerifierInstall: verification.VerifierInstallPathOnly,
	}
	version := legacyVersion
	if version == "" {
		version, err := bootstrap.resolveVerifierInstallVersion(owner, repo)
		if err != nil {
			return fmt.Errorf("%w: resolve verifier %s version: %w", verification.ErrVerifierCommandRequired, name, err)
		}
		return r.runBootstrapVerifier(ctx, &verifierBootstrapRequest{name: name, version: version, owner: owner, repo: repo, installer: bootstrap, args: args})
	}
	return r.runBootstrapVerifier(ctx, &verifierBootstrapRequest{name: name, version: version, owner: owner, repo: repo, installer: bootstrap, args: args})
}

type verifierBootstrapRequest struct {
	name      string
	version   string
	owner     string
	repo      string
	installer Installer
	args      []string
}

func (r verifierCommandRunner) runBootstrapVerifier(ctx context.Context, request *verifierBootstrapRequest) error {
	// Keeping installation and invocation in one helper makes it explicit that
	// latest and pinned compatibility versions follow the same execution path.
	binaryPath, err := request.installer.Install(request.owner, request.repo, request.version)
	if err != nil {
		return fmt.Errorf("%w: install verifier %s: %w", verification.ErrVerifierCommandRequired, request.name, err)
	}
	return runTrustedVerifier(ctx, binaryPath, r.policy, func() error {
		return runVerifierCommand(ctx, binaryPath, request.args...)
	})
}

// legacyVerifierVersion returns a compatible bootstrap version only for
// signature formats that Cosign v3 has deprecated. The flags are intentionally
// detected by shape, not URL: Atmos materializes remote sidecars before
// running Cosign, so their values are local paths by this point.
func legacyVerifierVersion(name string, args []string) string {
	if name != "cosign" {
		return ""
	}
	var certificate, signature bool
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--certificate":
			certificate = true
		case "--signature":
			signature = true
		}
	}
	if certificate && signature {
		return legacyCosignVerifierVersion
	}
	return ""
}

// trustVerifierBinaryFunc indirects trustVerifierBinary so tests can observe
// and override the platform-specific trust step without depending on
// darwin-only behavior.
var trustVerifierBinaryFunc = trustVerifierBinary

// runTrustedVerifier serializes bootstrap verifier execution. Trust repair is
// performed at most once per installed binary: repeatedly re-signing a shared
// executable makes macOS re-evaluate it and can itself interrupt a running
// verifier. The persistent sibling lock and marker coordinate independent
// Atmos processes using the same cache.
func runTrustedVerifier(ctx context.Context, binaryPath string, policy verification.Policy, run func() error) error {
	return filelock.New(binaryPath+".run.lock").WithExclusive(ctx, func() error {
		trustMarkerPath := binaryPath + ".trusted"
		if policy.VerifierTrust != verification.VerifierTrustDisabled && !fileExists(trustMarkerPath) {
			if trustErr := trustVerifierBinaryFunc(binaryPath); trustErr != nil {
				log.Warn("Could not mark downloaded verifier binary as locally trusted; the next command may fail",
					"path", binaryPath, "error", trustErr)
			} else if markerErr := os.WriteFile(trustMarkerPath, nil, defaultFileWritePermissions); markerErr != nil {
				log.Warn("Could not record local verifier trust state; it will be retried before the next command",
					"path", binaryPath, "error", markerErr)
			}
		}
		return run()
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (i *Installer) resolveVerifierInstallVersion(owner, repo string) (string, error) {
	var lookupErrs []error

	if i.useConfiguredReg {
		latest, err := latestVerifierVersion(i.configuredReg, owner, repo, "configured registry")
		if latest != "" {
			return latest, nil
		}
		if err != nil {
			lookupErrs = append(lookupErrs, err)
		}
	}

	latest, err := latestVerifierVersion(i.aquaVerifierRegistry(), owner, repo, "aqua registry")
	if latest != "" {
		return latest, nil
	}
	if err != nil {
		lookupErrs = append(lookupErrs, err)
	}

	if version, ok := fallbackVerifierInstallVersion(owner, repo); ok && len(lookupErrs) > 0 {
		log.Debug(
			"Using fallback verifier bootstrap version after latest lookup failure",
			logFieldOwner, owner,
			logFieldRepo, repo,
			logFieldVersion, version,
			"lookup_errors", errors.Join(lookupErrs...),
		)
		return version, nil
	}

	return "", verifierVersionUnavailableError(owner, repo, lookupErrs)
}

func fallbackVerifierInstallVersion(owner, repo string) (string, bool) {
	if owner == "sigstore" && repo == "cosign" {
		return defaultCosignVerifierVersion, true
	}
	return "", false
}

func (i *Installer) aquaVerifierRegistry() registry.ToolRegistry {
	if i.registryFactory == nil {
		return nil
	}
	return i.registryFactory.NewAquaRegistry()
}

func latestVerifierVersion(reg registry.ToolRegistry, owner, repo, source string) (string, error) {
	if reg == nil {
		return "", nil
	}
	latest, err := reg.GetLatestVersion(owner, repo)
	if err != nil {
		return "", fmt.Errorf("%s latest version lookup failed: %w", source, err)
	}
	return latest, nil
}

func verifierVersionUnavailableError(owner, repo string, lookupErrs []error) error {
	if len(lookupErrs) > 0 {
		return fmt.Errorf("%w: %s/%s: %w", ErrVerifierVersionUnavailable, owner, repo, errors.Join(lookupErrs...))
	}
	return fmt.Errorf("%w: %s/%s", ErrVerifierVersionUnavailable, owner, repo)
}

func runVerifierCommand(ctx context.Context, path string, args ...string) error {
	// #nosec G204,G702 -- verifier path is discovered via PATH or installed by the toolchain.
	cmd := exec.CommandContext(ctx, path, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s %v: %w\n%s", verification.ErrSignatureFailed, path, args, err, string(output))
	}
	return nil
}

func verifierTool(name string) (owner, repo string, ok bool) {
	switch name {
	case "cosign":
		return "sigstore", "cosign", true
	case "slsa-verifier":
		return "slsa-framework", "slsa-verifier", true
	case "gh":
		return "cli", "cli", true
	case "minisign":
		return "jedisct1", "minisign", true
	default:
		return "", "", false
	}
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
		"builtinRegistryCount", len(i.registries))

	// Use configured registries from atmos.yaml if available.
	if tool, found := i.findToolInConfiguredRegistry(owner, repo, version); found {
		return tool, nil
	}

	// Fallback: Search through builtin registries.
	if tool, found := i.findToolInBuiltinRegistries(owner, repo, version); found {
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
		log.Debug("No configured registries available, using builtin registries",
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

// findToolInBuiltinRegistries searches through builtin registries.
func (i *Installer) findToolInBuiltinRegistries(owner, repo, version string) (*registry.Tool, bool) {
	log.Debug("Falling back to builtin registries", "count", len(i.registries))
	for _, reg := range i.registries {
		log.Debug("Searching builtin registry", "registry", reg)
		tool, err := i.searchRegistry(reg, owner, repo, version)
		if err == nil {
			log.Debug("Tool found in builtin registry", "registry", reg)
			return tool, true
		}
		log.Debug("Tool not found in builtin registry", "registry", reg, "error", err)
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

	if tool == "" {
		return "", "", fmt.Errorf("%w: empty tool specification", ErrInvalidToolSpec)
	}

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
	// Track whether the version dir pre-existed so a failed install of a fresh
	// version cleans up after itself (no orphaned partial install that would fool
	// FindBinaryPath), without clobbering a previously installed good copy.
	preExisting := dirExists(versionDir)
	if err := os.MkdirAll(versionDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create version directory: %w", ErrFileOperation, err)
	}

	// Determine the binary name using shared resolution logic.
	binaryName := resolveBinaryName(tool)

	// Ensure Windows executables have .exe extension.
	binaryName = EnsureWindowsExeExtension(binaryName)

	binaryPath := filepath.Join(versionDir, binaryName)

	// For now, just copy the file (simplified extraction)
	if err := i.simpleExtract(assetPath, binaryPath, tool); err != nil {
		if !preExisting {
			// Remove the partial install so a subsequent run does not treat the
			// orphaned binary as "installed".
			_ = os.RemoveAll(versionDir) // #nosec G703 -- versionDir is the installer-created version directory.
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrFileOperation, err)
	}

	// A flat install writes the primary binary at binaryPath; a onedir install
	// writes only the .pkg tree + manifest (never binaryPath). So if the binary
	// is at binaryPath, this install is flat: clear any stale onedir artifacts
	// from a prior same-version onedir install, which would otherwise shadow the
	// freshly installed flat binary through readOnedirManifest below.
	if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() {
		if err := finalizeFlatInstall(versionDir); err != nil {
			return "", err
		}
	}

	// Onedir (multi-file) installs record the real entrypoint path in a sidecar
	// manifest instead of exposing a root symlink (Atmos creates no symlinks of
	// its own; see onedir.go). Resolve the primary here so the caller (chmod,
	// mtime, lock file) targets the file that was actually installed. Flat
	// installs are unaffected: with no manifest present, binaryPath is returned.
	if path, ok := resolveManifestEntrypoint(versionDir, ""); ok {
		return path, nil
	}

	return binaryPath, nil
}

// GetBinDir returns the binary installation directory.
func (i *Installer) GetBinDir() string {
	defer perf.Track(nil, "installer.Installer.GetBinDir")()

	return i.binDir
}

// GetBinaryPath returns the path to a specific version of a binary.
// If binaryName is provided and non-empty, it names the desired entrypoint;
// otherwise the tool's primary entrypoint is used. Onedir (multi-file) installs
// resolve the entrypoint's real (nested) path through the sidecar manifest;
// flat installs fall back to the version-dir root, auto-detecting an executable
// or using the repo name.
func (i *Installer) GetBinaryPath(owner, repo, version, binaryName string) string {
	defer perf.Track(nil, "installer.Installer.GetBinaryPath")()

	versionDir := filepath.Join(i.binDir, owner, repo, version)

	// Onedir installs record each entrypoint's real path in a sidecar manifest
	// instead of exposing a root symlink (Atmos creates no symlinks of its own;
	// see onedir.go). Resolve through it first — an explicit name maps to its
	// manifest entry, an empty name to the primary — since the entrypoint lives
	// nested inside the preserved .pkg tree, not at the version-dir root.
	if path, ok := resolveManifestEntrypoint(versionDir, binaryName); ok {
		return path
	}

	// If binary name is explicitly provided, use it directly (flat layout).
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
				isExec = strings.HasSuffix(strings.ToLower(entry.Name()), windowsExeExt)
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

// GetBinaryPaths returns every installed entrypoint path for a version. A onedir
// (multi-file) package exposes multiple commands that may live in different
// directories inside the preserved .pkg tree, so all of them are returned (for
// example so callers can add each command's directory to PATH). It returns nil
// for a flat install (no manifest); callers should fall back to GetBinaryPath.
func (i *Installer) GetBinaryPaths(owner, repo, version string) []string {
	defer perf.Track(nil, "installer.Installer.GetBinaryPaths")()

	if version == "latest" {
		if actual, err := i.ReadLatestFile(owner, repo); err == nil {
			version = actual
		}
	}

	versionDir := filepath.Join(i.binDir, owner, repo, version)
	manifest, ok := readOnedirManifest(versionDir)
	if !ok || len(manifest.Entrypoints) == 0 {
		return nil
	}

	paths := make([]string, 0, len(manifest.Entrypoints))
	for _, rel := range manifest.Entrypoints {
		paths = append(paths, filepath.Join(versionDir, rel))
	}
	sort.Strings(paths)
	return paths
}

// Uninstall removes a previously installed tool.
func (i *Installer) Uninstall(owner, repo, version string) error {
	defer perf.Track(nil, "toolchain.Installer.Uninstall")()

	versionDir := filepath.Join(i.binDir, owner, repo, version)
	lock := filelock.New(versionDir + ".lock")
	return lock.WithExclusive(context.Background(), func() error {
		binaryPath, err := i.FindBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("%w: tool %s/%s@%s is not installed", ErrToolNotFound, owner, repo, version)
		}

		binaryDir := versionDirFromBinaryPath(i.binDir, binaryPath)
		if err := os.RemoveAll(binaryDir); err != nil {
			return fmt.Errorf("%w: failed to remove %s: %w", ErrFileOperation, binaryDir, err)
		}

		for parentDir := filepath.Dir(binaryDir); parentDir != i.binDir && parentDir != "."; parentDir = filepath.Dir(parentDir) {
			if os.Remove(parentDir) != nil {
				break
			}
		}

		log.Debug("Successfully uninstalled tool", logFieldOwner, owner, logFieldRepo, repo, "version", version)
		//nolint:nilerr // Parent-directory cleanup is deliberately best-effort after uninstall succeeds.
		return nil
	})
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
