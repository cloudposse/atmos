package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Installer handles automatic tool installation.
type Installer struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewInstaller creates a new tool installer.
func NewInstaller(atmosConfig *schema.AtmosConfiguration) *Installer {
	defer perf.Track(nil, "dependencies.NewInstaller")()

	return &Installer{
		atmosConfig: atmosConfig,
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

	// Get tools directory
	toolsDir := i.atmosConfig.Toolchain.InstallPath
	if toolsDir == "" {
		toolsDir = ".tools"
	}

	// Resolve tool to owner/repo
	owner, repo, err := resolveToolPath(tool)
	if err != nil {
		return false
	}

	// Check if binary exists
	binaryPath := filepath.Join(toolsDir, "bin", owner, repo, version, repo)
	return fileExists(binaryPath)
}

// resolveToolPath resolves a tool name to owner/repo format.
// Handles both aliases (terraform) and full paths (hashicorp/terraform).
func resolveToolPath(tool string) (string, string, error) {
	// If already in owner/repo format
	if strings.Contains(tool, "/") {
		parts := strings.Split(tool, "/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("%w: %s", registry.ErrInvalidToolSpec, tool)
		}
		return parts[0], parts[1], nil
	}

	// Fallback: assume tool name is same as repo name with unknown owner
	// This will fail during install, but we try anyway
	return "unknown", tool, nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// UpdatePathForTools updates PATH environment variable to include tool binaries.
func UpdatePathForTools(atmosConfig *schema.AtmosConfiguration, dependencies map[string]string) error {
	defer perf.Track(atmosConfig, "dependencies.UpdatePathForTools")()

	if len(dependencies) == 0 {
		return nil
	}

	toolsDir := atmosConfig.Toolchain.InstallPath
	if toolsDir == "" {
		toolsDir = ".tools"
	}

	var paths []string
	for tool, version := range dependencies {
		// Resolve tool to owner/repo
		owner, repo, err := resolveToolPath(tool)
		if err != nil {
			continue // Skip invalid tools
		}

		// Add versioned bin directory to PATH
		binPath := filepath.Join(toolsDir, "bin", owner, repo, version)
		paths = append(paths, binPath)
	}

	if len(paths) == 0 {
		return nil
	}

	// Prepend to existing PATH
	// Use viper which checks environment variables automatically.
	currentPath := viper.GetString("PATH")
	newPath := strings.Join(append(paths, currentPath), string(os.PathListSeparator))
	os.Setenv("PATH", newPath)

	return nil
}
