package dependencies

import (
	"fmt"
	"os"
	execPkg "os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/toolchain/registry/aqua"
)

// InstallFunc is the function signature for installing a tool.
// The showHint parameter controls whether to show the PATH export hint message.
// The showProgressBar parameter controls whether to show spinner and success messages.
type InstallFunc func(toolSpec string, setAsDefault, reinstallFlag, showHint, showProgressBar bool) error

// BatchInstallFunc is the function signature for batch installing multiple tools.
// Shows status messages scrolling up with a single progress bar at bottom.
type BatchInstallFunc func(toolSpecs []string, reinstallFlag bool) error

// BinaryPathFinder finds installed tool binaries.
// This interface allows testing without the full toolchain installer.
type BinaryPathFinder interface {
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
}

// VersionLister fetches available versions for a tool from its release source.
// This interface decouples constraint resolution from the concrete registry implementation.
type VersionLister interface {
	GetAvailableVersions(owner, repo string) ([]string, error)
}

// Installer handles automatic tool installation.
type Installer struct {
	atmosConfig      *schema.AtmosConfiguration
	resolver         toolchain.ToolResolver
	installFunc      InstallFunc
	batchInstallFunc BatchInstallFunc
	fileExistsFunc   func(path string) bool
	binaryPathFinder BinaryPathFinder
	versionLister    VersionLister
}

// InstallerOption is a functional option for configuring Installer.
type InstallerOption func(*Installer)

// WithResolver sets a custom ToolResolver (for testing).
func WithResolver(resolver toolchain.ToolResolver) InstallerOption {
	defer perf.Track(nil, "dependencies.WithResolver")()

	return func(i *Installer) {
		i.resolver = resolver
	}
}

// WithInstallFunc sets a custom install function (for testing).
func WithInstallFunc(fn InstallFunc) InstallerOption {
	defer perf.Track(nil, "dependencies.WithInstallFunc")()

	return func(i *Installer) {
		i.installFunc = fn
	}
}

// WithBatchInstallFunc sets a custom batch install function (for testing).
func WithBatchInstallFunc(fn BatchInstallFunc) InstallerOption {
	defer perf.Track(nil, "dependencies.WithBatchInstallFunc")()

	return func(i *Installer) {
		i.batchInstallFunc = fn
	}
}

// WithFileExistsFunc sets a custom file exists function (for testing).
func WithFileExistsFunc(fn func(path string) bool) InstallerOption {
	defer perf.Track(nil, "dependencies.WithFileExistsFunc")()

	return func(i *Installer) {
		i.fileExistsFunc = fn
	}
}

// WithBinaryPathFinder sets a custom binary path finder (for testing).
func WithBinaryPathFinder(finder BinaryPathFinder) InstallerOption {
	defer perf.Track(nil, "dependencies.WithBinaryPathFinder")()

	return func(i *Installer) {
		i.binaryPathFinder = finder
	}
}

// WithVersionLister sets a custom version lister (for testing).
func WithVersionLister(lister VersionLister) InstallerOption {
	defer perf.Track(nil, "dependencies.WithVersionLister")()

	return func(i *Installer) {
		i.versionLister = lister
	}
}

// NewInstaller creates a new tool installer.
func NewInstaller(atmosConfig *schema.AtmosConfiguration, opts ...InstallerOption) *Installer {
	defer perf.Track(nil, "dependencies.NewInstaller")()

	// Use the same path logic as toolchain installation to ensure binary detection
	// uses the same directory where tools are actually installed.
	// Honor explicit InstallPath from passed config; otherwise use toolchain's default path.
	toolsDir := toolchain.GetInstallPath()
	if atmosConfig != nil && atmosConfig.Toolchain.InstallPath != "" {
		toolsDir = atmosConfig.Toolchain.InstallPath
	}
	binDir := filepath.Join(toolsDir, "bin")

	// Create toolchain installer with correct binDir for binary path detection.
	tcInstaller := toolchain.NewInstallerWithBinDir(binDir)

	inst := &Installer{
		atmosConfig:      atmosConfig,
		resolver:         tcInstaller.GetResolver(),
		installFunc:      toolchain.RunInstall,
		batchInstallFunc: toolchain.RunInstallBatch,
		fileExistsFunc:   fileExists,
		binaryPathFinder: tcInstaller, // Uses FindBinaryPath() for proper binary detection.
		versionLister:    aqua.NewAquaRegistry(),
	}

	// Apply options.
	for _, opt := range opts {
		opt(inst)
	}

	return inst
}

// EnsureTools ensures all required tools are installed.
// Installs missing tools automatically using batch install with progress bar.
// Version constraints (e.g., "^1.10.0", "~> 1.5") are resolved to concrete
// versions before installation. The dependency map is updated in-place with
// resolved versions so downstream callers (PATH building, binary lookup) use
// concrete version strings.
func (i *Installer) EnsureTools(dependencies map[string]string) error {
	defer perf.Track(i.atmosConfig, "dependencies.EnsureTools")()

	if len(dependencies) == 0 {
		return nil
	}

	// Resolve any semver constraints to concrete versions before install.
	if err := i.resolveConstraints(dependencies); err != nil {
		return err
	}

	// Collect missing tools.
	var missingTools []string
	for tool, version := range dependencies {
		if !i.isToolInstalled(tool, version) {
			missingTools = append(missingTools, fmt.Sprintf("%s@%s", tool, version))
		}
	}

	if len(missingTools) == 0 {
		return nil
	}

	// Batch install all missing tools with progress bar.
	if err := i.batchInstallFunc(missingTools, false); err != nil {
		return errUtils.Build(errUtils.ErrToolInstall).
			WithCause(err).
			WithExplanation("Failed to install dependencies").
			Err()
	}

	return nil
}

// resolveConstraints replaces semver constraint strings in the dependency map
// with the highest concrete version that satisfies each constraint.
// For example, "^1.10.0" might resolve to "1.10.3".
func (i *Installer) resolveConstraints(deps map[string]string) error {
	defer perf.Track(i.atmosConfig, "dependencies.resolveConstraints")()

	for tool, version := range deps {
		if !isConstraint(version) {
			continue
		}

		// Parse the constraint.
		constraint, err := semver.NewConstraint(version)
		if err != nil {
			return errUtils.Build(errUtils.ErrDependencyConstraint).
				WithCause(err).
				WithExplanationf("Invalid version constraint %q for tool %q", version, tool).
				Err()
		}

		// Resolve tool name to owner/repo for the version listing API.
		owner, repo, err := i.resolver.Resolve(tool)
		if err != nil {
			return errUtils.Build(errUtils.ErrDependencyResolution).
				WithCause(err).
				WithExplanationf("Cannot resolve tool %q to owner/repo for constraint resolution", tool).
				Err()
		}

		// Fetch available versions from the registry.
		available, err := i.versionLister.GetAvailableVersions(owner, repo)
		if err != nil {
			return errUtils.Build(errUtils.ErrDependencyResolution).
				WithCause(err).
				WithExplanationf("Failed to fetch available versions for %s/%s", owner, repo).
				WithHintf("Check your network connection or try specifying a concrete version instead of %q", version).
				Err()
		}

		// Find the highest version satisfying the constraint.
		resolved, err := highestMatch(available, constraint)
		if err != nil {
			return errUtils.Build(errUtils.ErrDependencyConstraint).
				WithCause(err).
				WithExplanationf("No available version of %s/%s satisfies constraint %q", owner, repo, version).
				WithHint("Run `atmos toolchain search` to see available versions").
				Err()
		}

		// Update the map in-place so downstream code uses the concrete version.
		deps[tool] = resolved
	}

	return nil
}

// isConstraint returns true if the version string is a semver constraint
// rather than a concrete version. Constraints contain operators like ^, ~, >, <, =, ||, or spaces.
func isConstraint(version string) bool {
	if version == "" || version == "latest" {
		return false
	}
	// A concrete version parses successfully as a semver version.
	// If it fails, but succeeds as a constraint, it's a constraint.
	_, err := semver.NewVersion(strings.TrimPrefix(version, "v"))
	return err != nil
}

// highestMatch finds the highest version from candidates that satisfies the constraint.
func highestMatch(candidates []string, constraint *semver.Constraints) (string, error) {
	var matched []*semver.Version
	for _, c := range candidates {
		v, err := semver.NewVersion(strings.TrimPrefix(c, "v"))
		if err != nil {
			continue // Skip unparseable versions.
		}
		if constraint.Check(v) {
			matched = append(matched, v)
		}
	}

	if len(matched) == 0 {
		return "", errUtils.ErrDependencyConstraint
	}

	// Sort descending and pick the highest.
	sort.Sort(sort.Reverse(semver.Collection(matched)))
	return matched[0].Original(), nil
}

// ensureTool ensures a specific tool version is installed.
func (i *Installer) ensureTool(tool string, version string) error {
	defer perf.Track(i.atmosConfig, "dependencies.ensureTool")()

	// Check if already installed.
	if i.isToolInstalled(tool, version) {
		return nil
	}

	// Install missing tool. Pass showHint=false to suppress PATH hint, showProgressBar=true for spinner.
	toolSpec := fmt.Sprintf("%s@%s", tool, version)
	if err := i.installFunc(toolSpec, false, false, false, true); err != nil {
		return errUtils.Build(errUtils.ErrToolInstall).
			WithCause(err).
			WithExplanationf("Failed to install %s", toolSpec).
			Err()
	}

	return nil
}

// isToolInstalled checks if a tool version is already installed.
// Uses FindBinaryPath for proper binary detection, which handles:
// - Binaries with different names than the repo (e.g., opentofu -> tofu)
// - Files[0].Name from registry configuration
// - Auto-detection of executables in the version directory.
func (i *Installer) isToolInstalled(tool string, version string) bool {
	defer perf.Track(i.atmosConfig, "dependencies.Installer.isToolInstalled")()

	// Resolve tool to owner/repo using the shared resolver.
	owner, repo, err := i.resolver.Resolve(tool)
	if err != nil {
		return false
	}

	// Use FindBinaryPath for proper binary detection.
	// This handles tools where binary name differs from repo name.
	_, err = i.binaryPathFinder.FindBinaryPath(owner, repo, version)
	return err == nil
}

// ResolveExecutablePath resolves a bare executable name (e.g., "tofu") to its absolute path
// using the toolchain dependency map. This is needed when the executable is installed via
// `atmos toolchain install` and is not on the system PATH.
//
// Resolution order:
//  1. For each dependency, check if the toolchain-installed binary matches the executable name.
//  2. Fall back to exec.LookPath to check the system PATH.
//  3. If nothing found, return the original name (let the caller fail with a clear error).
func (i *Installer) ResolveExecutablePath(deps map[string]string, executable string) string {
	defer perf.Track(i.atmosConfig, "dependencies.Installer.ResolveExecutablePath")()

	// If the executable is already an absolute path, return it as-is.
	if filepath.IsAbs(executable) {
		return executable
	}

	// Try to find the executable in toolchain-installed dependencies.
	for tool, version := range deps {
		owner, repo, err := i.resolver.Resolve(tool)
		if err != nil {
			continue
		}

		binaryPath, err := i.binaryPathFinder.FindBinaryPath(owner, repo, version)
		if err != nil {
			continue
		}

		// Normalize names so bare executables still match platform-specific suffixes (e.g. .exe on Windows).
		binaryBase := strings.TrimSuffix(filepath.Base(binaryPath), filepath.Ext(binaryPath))
		executableBase := strings.TrimSuffix(executable, filepath.Ext(executable))
		if binaryBase == executableBase {
			return binaryPath
		}
	}

	// Fall back to system PATH lookup.
	if path, err := execPkg.LookPath(executable); err == nil {
		return path
	}

	// Return the original name; the caller will get a clear error from tfexec.NewTerraform.
	return executable
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getPathFromEnv retrieves PATH from environment.
func getPathFromEnv() string {
	return os.Getenv("PATH") //nolint:forbidigo // Reading PATH env var directly is intentional here
}

// BuildToolchainPATH constructs a PATH string with toolchain binaries prepended.
// This function does NOT modify the global environment - it returns the PATH string
// which should be added to ComponentEnvList for subprocess execution.
func BuildToolchainPATH(atmosConfig *schema.AtmosConfiguration, dependencies map[string]string) (string, error) {
	defer perf.Track(atmosConfig, "dependencies.BuildToolchainPATH")()

	if len(dependencies) == 0 {
		return getPathFromEnv(), nil
	}

	// Use the same path logic as toolchain installation to ensure PATH points
	// to where tools are actually installed (XDG by default, or configured path).
	// Honor explicit InstallPath from passed config; otherwise use toolchain's default path.
	toolsDir := toolchain.GetInstallPath()
	if atmosConfig != nil && atmosConfig.Toolchain.InstallPath != "" {
		toolsDir = atmosConfig.Toolchain.InstallPath
	}

	// Get the resolver from toolchain package for alias and registry resolution.
	tcInstaller := toolchain.NewInstaller()
	resolver := tcInstaller.GetResolver()

	var paths []string
	for tool, version := range dependencies {
		// Resolve tool to owner/repo using the shared resolver.
		owner, repo, err := resolver.Resolve(tool)
		if err != nil {
			continue // Skip invalid tools.
		}

		// Add versioned bin directory to PATH.
		binPath := filepath.Join(toolsDir, "bin", owner, repo, version)

		// Convert to absolute path to avoid Go 1.19+ exec.LookPath security issues.
		// Go 1.19+ rejects executables found via relative PATH entries.
		// Note: filepath.Abs rarely fails in practice; we trust it to succeed here.
		absBinPath, _ := filepath.Abs(binPath)

		paths = append(paths, absBinPath)
	}

	// Prepend toolchain paths to existing PATH.
	currentPath := getPathFromEnv()
	if len(paths) == 0 {
		return currentPath, nil
	}

	newPath := strings.Join(append(paths, currentPath), string(os.PathListSeparator))
	return newPath, nil
}

// UpdatePathForTools updates PATH environment variable to include tool binaries.
// Deprecated: Use BuildToolchainPATH() instead and add to ComponentEnvList.
// This function is kept for backwards compatibility but should not be used.
func UpdatePathForTools(atmosConfig *schema.AtmosConfiguration, dependencies map[string]string) error {
	defer perf.Track(atmosConfig, "dependencies.UpdatePathForTools")()

	newPath, err := BuildToolchainPATH(atmosConfig, dependencies)
	if err != nil {
		return err
	}

	// Only set if PATH actually changed.
	if newPath != getPathFromEnv() {
		os.Setenv("PATH", newPath)
	}

	return nil
}
