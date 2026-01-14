package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
)

// InstallFunc is the function signature for installing a tool.
// The showHint parameter controls whether to show the PATH export hint message.
// The showProgressBar parameter controls whether to show spinner and success messages.
type InstallFunc func(toolSpec string, setAsDefault, reinstallFlag, showHint, showProgressBar bool) error

// BinaryPathFinder finds installed tool binaries.
// This interface allows testing without the full toolchain installer.
type BinaryPathFinder interface {
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
}

// Installer handles automatic tool installation.
type Installer struct {
	atmosConfig      *schema.AtmosConfiguration
	resolver         toolchain.ToolResolver
	installFunc      InstallFunc
	fileExistsFunc   func(path string) bool
	binaryPathFinder BinaryPathFinder
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

// NewInstaller creates a new tool installer.
func NewInstaller(atmosConfig *schema.AtmosConfiguration, opts ...InstallerOption) *Installer {
	defer perf.Track(nil, "dependencies.NewInstaller")()

	// Determine binDir based on config or default.
	toolsDir := ".tools"
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
		fileExistsFunc:   fileExists,
		binaryPathFinder: tcInstaller, // Uses FindBinaryPath() for proper binary detection.
	}

	// Apply options.
	for _, opt := range opts {
		opt(inst)
	}

	return inst
}

// EnsureTools ensures all required tools are installed.
// Installs missing tools automatically.
func (i *Installer) EnsureTools(dependencies map[string]string) error {
	defer perf.Track(i.atmosConfig, "dependencies.EnsureTools")()

	if len(dependencies) == 0 {
		return nil
	}

	for tool, version := range dependencies {
		if err := i.ensureTool(tool, version); err != nil {
			return err
		}
	}

	return nil
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

	// Guard against nil atmosConfig.
	toolsDir := ".tools"
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
		paths = append(paths, binPath)
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
