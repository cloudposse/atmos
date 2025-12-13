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

// Installer handles automatic tool installation.
type Installer struct {
	atmosConfig *schema.AtmosConfiguration
	resolver    toolchain.ToolResolver
}

// NewInstaller creates a new tool installer.
func NewInstaller(atmosConfig *schema.AtmosConfiguration) *Installer {
	defer perf.Track(nil, "dependencies.NewInstaller")()

	// Get the resolver from toolchain package for alias and registry resolution.
	tcInstaller := toolchain.NewInstaller()

	return &Installer{
		atmosConfig: atmosConfig,
		resolver:    tcInstaller.GetResolver(),
	}
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

	// Check if already installed
	if i.isToolInstalled(tool, version) {
		return nil
	}

	// Install missing tool
	toolSpec := fmt.Sprintf("%s@%s", tool, version)
	if err := toolchain.RunInstall(toolSpec, false, false); err != nil {
		return fmt.Errorf("%w: failed to install %s: %w", errUtils.ErrToolInstall, toolSpec, err)
	}

	return nil
}

// isToolInstalled checks if a tool version is already installed.
func (i *Installer) isToolInstalled(tool string, version string) bool {
	defer perf.Track(i.atmosConfig, "dependencies.Installer.isToolInstalled")()

	// Get tools directory.
	toolsDir := ".tools"
	if i.atmosConfig != nil && i.atmosConfig.Toolchain.InstallPath != "" {
		toolsDir = i.atmosConfig.Toolchain.InstallPath
	}

	// Resolve tool to owner/repo using the shared resolver.
	owner, repo, err := i.resolver.Resolve(tool)
	if err != nil {
		return false
	}

	// Check if binary exists.
	binaryPath := filepath.Join(toolsDir, "bin", owner, repo, version, repo)
	return fileExists(binaryPath)
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
